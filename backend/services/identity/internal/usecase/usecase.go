// Package usecase — сценарии аккаунта и сессий.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"github.com/google/uuid"

	"plenka/identity/internal/core"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

type Accounts interface {
	Insert(ctx context.Context, a core.Account) error
	ByEmail(ctx context.Context, email string) (core.Account, error)
	ByID(ctx context.Context, id uuid.UUID) (core.Account, error)
	UpdateProfile(ctx context.Context, a core.Account) (core.Account, error) // оптимистично по revision
	SetPassword(ctx context.Context, id uuid.UUID, hash string) error
	AdminExists(ctx context.Context) (bool, error)
}

type Sessions interface {
	Insert(ctx context.Context, s core.Session, tokenHash []byte) error
	ByToken(ctx context.Context, tokenHash []byte) (core.Session, error)
	Touch(ctx context.Context, id uuid.UUID, now, idleUntil time.Time) error
	Revoke(ctx context.Context, account, id uuid.UUID, now time.Time) error
	RevokeByToken(ctx context.Context, tokenHash []byte, now time.Time) error
	RevokeOthers(ctx context.Context, account, keep uuid.UUID, now time.Time) error
	ListAlive(ctx context.Context, account uuid.UUID, now time.Time) ([]core.Session, error)
}

type Hasher interface {
	Hash(pw string) (string, error)
	Verify(hash, pw string) bool
}

type Service struct {
	Accounts Accounts
	Sessions Sessions
	Hasher   Hasher
	Policy   core.Policy
	Clock    func() time.Time

	once  sync.Once
	dummy string
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login — результат входа: токен уходит в cookie, аккаунт — в ответ.
type Login struct {
	Token   string
	Expires time.Time
	Account core.Account
	Session uuid.UUID
}

type Device struct{ UserAgent, IP string }

func (s *Service) Register(ctx context.Context, d core.Draft, dev Device) (Login, error) {
	f := problem.Fields{}
	email := core.NormalizeEmail(d.Email, f)
	core.CheckPassword(d.Password, email, f)
	name := core.NormalizeName(d.DisplayName, f)
	birth := core.ParseBirthDate(d.BirthDate, s.now(), f)
	if err := f.Err(); err != nil {
		return Login{}, err
	}
	hash, err := s.Hasher.Hash(d.Password)
	if err != nil {
		return Login{}, err
	}
	a := core.Account{ID: uuid.New(), Email: email, PasswordHash: hash, DisplayName: name, BirthDate: birth,
		Role: principal.Viewer, CreatedAt: s.now(), Revision: 1}
	if err := s.Accounts.Insert(ctx, a); err != nil {
		return Login{}, err
	}
	return s.open(ctx, a, dev)
}

func (s *Service) Login(ctx context.Context, email, pw string, dev Device) (Login, error) {
	fail := problem.New(401, "bad_credentials", "Неверный email или пароль")
	a, err := s.Accounts.ByEmail(ctx, core.NormalizeEmail(email, problem.Fields{}))
	if problem.Is(err, "not_found") {
		s.Hasher.Verify(s.placeholder(), pw) // одинаковое время ответа
		return Login{}, fail
	}
	if err != nil {
		return Login{}, err
	}
	if !s.Hasher.Verify(a.PasswordHash, pw) {
		return Login{}, fail
	}
	return s.open(ctx, a, dev)
}

// placeholder — настоящий хэш (с теми же параметрами), чтобы проверка для
// несуществующего email занимала столько же времени, сколько для реального.
func (s *Service) placeholder() string {
	s.once.Do(func() { s.dummy, _ = s.Hasher.Hash("not-a-real-password-1") })
	return s.dummy
}

func (s *Service) open(ctx context.Context, a core.Account, dev Device) (Login, error) {
	now := s.now()
	tok := newToken()
	sess := core.Session{ID: uuid.New(), AccountID: a.ID, UserAgent: trim(dev.UserAgent, 200), IP: dev.IP,
		CreatedAt: now, LastSeenAt: now, IdleExpires: now.Add(s.Policy.Idle), AbsExpires: now.Add(s.Policy.Absolute)}
	if err := s.Sessions.Insert(ctx, sess, HashToken(tok)); err != nil {
		return Login{}, err
	}
	return Login{Token: tok, Expires: sess.AbsExpires, Account: a, Session: sess.ID}, nil
}

// Resolve — по токену из cookie: кто это. Вызывается gateway по gRPC.
// Скользящий срок продлевается не чаще Policy.Touch, чтобы не писать в БД
// на каждый запрос.
func (s *Service) Resolve(ctx context.Context, token string) (core.Account, core.Session, bool, error) {
	if token == "" {
		return core.Account{}, core.Session{}, false, nil
	}
	sess, err := s.Sessions.ByToken(ctx, HashToken(token))
	if problem.Is(err, "not_found") {
		return core.Account{}, core.Session{}, false, nil
	}
	if err != nil {
		return core.Account{}, core.Session{}, false, err
	}
	now := s.now()
	if !sess.Alive(now) {
		return core.Account{}, core.Session{}, false, nil
	}
	if sess.NeedsTouch(now, s.Policy) {
		sess.IdleExpires = sess.Extended(now, s.Policy)
		if err := s.Sessions.Touch(ctx, sess.ID, now, sess.IdleExpires); err != nil {
			return core.Account{}, core.Session{}, false, err
		}
	}
	a, err := s.Accounts.ByID(ctx, sess.AccountID)
	if err != nil {
		return core.Account{}, core.Session{}, false, err
	}
	return a, sess, true, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.Sessions.RevokeByToken(ctx, HashToken(token), s.now())
}

func (s *Service) Me(ctx context.Context) (core.Account, error) {
	p, err := principal.Need(ctx)
	if err != nil {
		return core.Account{}, err
	}
	return s.Accounts.ByID(ctx, uuid.MustParse(p.UserID))
}

func (s *Service) UpdateProfile(ctx context.Context, name, birth string, revision int64) (core.Account, error) {
	a, err := s.Me(ctx)
	if err != nil {
		return a, err
	}
	f := problem.Fields{}
	a.DisplayName = core.NormalizeName(name, f)
	a.BirthDate = core.ParseBirthDate(birth, s.now(), f)
	if err := f.Err(); err != nil {
		return a, err
	}
	if revision != 0 && revision != a.Revision {
		return a, problem.Conflict("stale", "Профиль изменён на другом устройстве — обновите страницу")
	}
	return s.Accounts.UpdateProfile(ctx, a)
}

// ChangePassword — меняет пароль и закрывает все остальные сессии.
func (s *Service) ChangePassword(ctx context.Context, currentSession uuid.UUID, oldPw, newPw string) error {
	a, err := s.Me(ctx)
	if err != nil {
		return err
	}
	if !s.Hasher.Verify(a.PasswordHash, oldPw) {
		return problem.Invalid(map[string]string{"current_password": "Текущий пароль неверен"})
	}
	f := problem.Fields{}
	core.CheckPassword(newPw, a.Email, f)
	if err := f.Err(); err != nil {
		return err
	}
	hash, err := s.Hasher.Hash(newPw)
	if err != nil {
		return err
	}
	if err := s.Accounts.SetPassword(ctx, a.ID, hash); err != nil {
		return err
	}
	return s.Sessions.RevokeOthers(ctx, a.ID, currentSession, s.now())
}

func (s *Service) Devices(ctx context.Context) ([]core.Session, error) {
	p, err := principal.Need(ctx)
	if err != nil {
		return nil, err
	}
	return s.Sessions.ListAlive(ctx, uuid.MustParse(p.UserID), s.now())
}

// RevokeDevice — «выйти на этом устройстве» из личного кабинета. Действует
// мгновенно: следующий запрос с этой cookie уже не пройдёт (с поправкой на
// кэш gateway, см. TRADEOFFS).
func (s *Service) RevokeDevice(ctx context.Context, id uuid.UUID) error {
	p, err := principal.Need(ctx)
	if err != nil {
		return err
	}
	return s.Sessions.Revoke(ctx, uuid.MustParse(p.UserID), id, s.now())
}

// EnsureAdmin — первый администратор из окружения.
func (s *Service) EnsureAdmin(ctx context.Context, email, pw string) (bool, error) {
	ok, err := s.Accounts.AdminExists(ctx)
	if err != nil || ok || email == "" {
		return false, err
	}
	hash, err := s.Hasher.Hash(pw)
	if err != nil {
		return false, err
	}
	a := core.Account{ID: uuid.New(), Email: email, PasswordHash: hash, DisplayName: "Администратор",
		Role: principal.Admin, CreatedAt: s.now(), Revision: 1}
	if err := s.Accounts.Insert(ctx, a); err != nil {
		if problem.Is(err, "email_taken") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
