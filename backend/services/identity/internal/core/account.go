// Package core — аккаунт зрителя и правила его данных.
package core

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"plenka/kit/principal"
	"plenka/kit/problem"
)

type Account struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	DisplayName  string
	BirthDate    *time.Time
	Role         principal.Role
	CreatedAt    time.Time
	Revision     int64
}

func (a Account) BirthYear() int {
	if a.BirthDate == nil {
		return 0
	}
	return a.BirthDate.Year()
}

// Draft — данные регистрации и профиля до проверки.
type Draft struct {
	Email       string
	Password    string
	DisplayName string
	BirthDate   string // ГГГГ-ММ-ДД или пусто
}

// NormalizeEmail — нижний регистр, без пробелов, валидный адрес.
func NormalizeEmail(raw string, f problem.Fields) string {
	e := strings.ToLower(strings.TrimSpace(raw))
	if a, err := mail.ParseAddress(e); err != nil || a.Address != e || !strings.Contains(e[strings.IndexByte(e, '@')+1:], ".") {
		f.Set("email", "Введите email вида name@example.ru")
	}
	return e
}

// CheckPassword — от 10 символов, буквы и цифры, не совпадает с email.
// Длина важнее состава: 10 символов — компромисс между стойкостью и тем,
// что зритель вводит пароль на телевизоре с пульта.
func CheckPassword(pw, email string, f problem.Fields) {
	n := utf8.RuneCountInString(pw)
	var letter, digit bool
	for _, r := range pw {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	switch {
	case n < 10:
		f.Set("password", "Пароль — минимум 10 символов")
	case n > 128:
		f.Set("password", "Пароль — не длиннее 128 символов")
	case !letter || !digit:
		f.Set("password", "В пароле нужны буквы и цифры")
	case email != "" && strings.EqualFold(pw, email):
		f.Set("password", "Пароль не должен совпадать с email")
	}
}

func NormalizeName(raw string, f problem.Fields) string {
	n := strings.Join(strings.Fields(raw), " ")
	if c := utf8.RuneCountInString(n); c < 2 || c > 60 {
		f.Set("display_name", "Имя — от 2 до 60 символов")
	}
	return n
}

// ParseBirthDate — пусто разрешено (тогда контент 18+ недоступен).
func ParseBirthDate(raw string, now time.Time, f problem.Fields) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		f.Set("birth_date", "Дата в формате ГГГГ-ММ-ДД")
		return nil
	}
	if d.Year() < 1900 || d.After(now) {
		f.Set("birth_date", "Проверьте дату рождения")
		return nil
	}
	if now.Year()-d.Year() < 6 {
		f.Set("birth_date", "Регистрация доступна с 6 лет")
		return nil
	}
	return &d
}

// Session — активный вход с устройства.
type Session struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	UserAgent   string
	IP          string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	IdleExpires time.Time
	AbsExpires  time.Time
	RevokedAt   *time.Time
}

// Policy — сроки жизни сессии.
type Policy struct {
	Idle     time.Duration // без активности
	Absolute time.Duration // максимум от входа
	Touch    time.Duration // не чаще этого продлевать (меньше записей в БД)
}

func DefaultPolicy() Policy {
	return Policy{Idle: 14 * 24 * time.Hour, Absolute: 30 * 24 * time.Hour, Touch: 5 * time.Minute}
}

// Alive — сессия действительна в момент now.
func (s Session) Alive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.IdleExpires) && now.Before(s.AbsExpires)
}

// NeedsTouch — пора ли продлить скользящий срок.
func (s Session) NeedsTouch(now time.Time, p Policy) bool { return now.Sub(s.LastSeenAt) >= p.Touch }

// Extended — новый скользящий срок, не позже абсолютного.
func (s Session) Extended(now time.Time, p Policy) time.Time {
	next := now.Add(p.Idle)
	if next.After(s.AbsExpires) {
		return s.AbsExpires
	}
	return next
}
