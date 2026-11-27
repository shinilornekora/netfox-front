// Package pgsql — хранилище аккаунтов и сессий + argon2id.
package pgsql

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"

	"plenka/identity/internal/core"
	"plenka/kit/pgkit"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

type Accounts struct{ DB *pgxpool.Pool }

const accCols = `id, email, password_hash, display_name, birth_date, role, created_at, revision`

func scanAcc(row pgx.Row) (core.Account, error) {
	var a core.Account
	var role string
	err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DisplayName, &a.BirthDate, &role, &a.CreatedAt, &a.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, problem.NotFound("Аккаунт")
	}
	a.Role = principal.Role(role)
	return a, err
}

func (s Accounts) Insert(ctx context.Context, a core.Account) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO accounts(`+accCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.Email, a.PasswordHash, a.DisplayName, a.BirthDate, string(a.Role), a.CreatedAt, a.Revision)
	if pgkit.Constraint(err) == "accounts_email_uq" {
		return problem.Conflict("email_taken", "Этот email уже зарегистрирован — попробуйте войти")
	}
	return err
}

func (s Accounts) ByEmail(ctx context.Context, email string) (core.Account, error) {
	return scanAcc(s.DB.QueryRow(ctx, `SELECT `+accCols+` FROM accounts WHERE email=$1`, email))
}

func (s Accounts) ByID(ctx context.Context, id uuid.UUID) (core.Account, error) {
	return scanAcc(s.DB.QueryRow(ctx, `SELECT `+accCols+` FROM accounts WHERE id=$1`, id))
}

func (s Accounts) UpdateProfile(ctx context.Context, a core.Account) (core.Account, error) {
	out, err := scanAcc(s.DB.QueryRow(ctx, `UPDATE accounts SET display_name=$3, birth_date=$4, revision=revision+1
		WHERE id=$1 AND revision=$2 RETURNING `+accCols, a.ID, a.Revision, a.DisplayName, a.BirthDate))
	if problem.Is(err, "not_found") {
		return out, problem.Conflict("stale", "Профиль изменён на другом устройстве — обновите страницу")
	}
	return out, err
}

func (s Accounts) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := s.DB.Exec(ctx, `UPDATE accounts SET password_hash=$2, revision=revision+1 WHERE id=$1`, id, hash)
	return err
}

func (s Accounts) AdminExists(ctx context.Context) (bool, error) {
	var ok bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE role='admin')`).Scan(&ok)
	return ok, err
}

// ---------- сессии ----------

type Sessions struct{ DB *pgxpool.Pool }

const sesCols = `id, account_id, user_agent, ip, created_at, last_seen_at, idle_expires_at, absolute_expires_at, revoked_at`

func scanSes(row pgx.Row) (core.Session, error) {
	var s core.Session
	err := row.Scan(&s.ID, &s.AccountID, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastSeenAt, &s.IdleExpires, &s.AbsExpires, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, problem.NotFound("Сессия")
	}
	return s, err
}

func (r Sessions) Insert(ctx context.Context, s core.Session, hash []byte) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO sessions(id, token_hash, account_id, user_agent, ip, created_at, last_seen_at, idle_expires_at, absolute_expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s.ID, hash, s.AccountID, s.UserAgent, s.IP, s.CreatedAt, s.LastSeenAt, s.IdleExpires, s.AbsExpires)
	return err
}

func (r Sessions) ByToken(ctx context.Context, hash []byte) (core.Session, error) {
	return scanSes(r.DB.QueryRow(ctx, `SELECT `+sesCols+` FROM sessions WHERE token_hash=$1`, hash))
}

func (r Sessions) Touch(ctx context.Context, id uuid.UUID, now, idle time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE sessions SET last_seen_at=$2, idle_expires_at=$3 WHERE id=$1 AND revoked_at IS NULL`, id, now, idle)
	return err
}

func (r Sessions) Revoke(ctx context.Context, account, id uuid.UUID, now time.Time) error {
	tag, err := r.DB.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE id=$1 AND account_id=$2 AND revoked_at IS NULL`, id, account, now)
	if err == nil && tag.RowsAffected() == 0 {
		return problem.NotFound("Сессия")
	}
	return err
}

func (r Sessions) RevokeByToken(ctx context.Context, hash []byte, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE token_hash=$1 AND revoked_at IS NULL`, hash, now)
	return err
}

func (r Sessions) RevokeOthers(ctx context.Context, account, keep uuid.UUID, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE account_id=$1 AND id<>$2 AND revoked_at IS NULL`, account, keep, now)
	return err
}

func (r Sessions) ListAlive(ctx context.Context, account uuid.UUID, now time.Time) ([]core.Session, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+sesCols+` FROM sessions WHERE account_id=$1 AND revoked_at IS NULL
		AND idle_expires_at > $2 AND absolute_expires_at > $2 ORDER BY last_seen_at DESC`, account, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Session{}
	for rows.Next() {
		s, err := scanSes(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------- argon2id ----------

// Argon2 — memory-hard хэш паролей (рекомендация OWASP), в отличие от bcrypt
// дорог для GPU-перебора. Параметры: 19 МиБ, 2 прохода, 1 поток.
type Argon2 struct {
	MemoryKiB uint32
	Time      uint32
}

func (a Argon2) params() (uint32, uint32) {
	m, t := a.MemoryKiB, a.Time
	if m == 0 {
		m = 19 * 1024
	}
	if t == 0 {
		t = 2
	}
	return m, t
}

func (a Argon2) Hash(pw string) (string, error) {
	m, t := a.params()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, t, m, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=1$%s$%s", m, t,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (Argon2) Verify(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
