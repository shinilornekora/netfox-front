package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"plenka/identity/internal/adapter/pgsql"
	"plenka/identity/internal/core"
	"plenka/identity/internal/usecase"
	"plenka/identity/migrations"
	"plenka/kit/pgtest"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*usecase.Service, *clock) {
	db := pgtest.Fresh(t, "identity_test", migrations.FS)
	c := &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	return &usecase.Service{Accounts: pgsql.Accounts{DB: db}, Sessions: pgsql.Sessions{DB: db},
		Hasher: pgsql.Argon2{MemoryKiB: 1024, Time: 1}, Policy: core.DefaultPolicy(), Clock: c.now}, c
}

func as(a core.Account) context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: a.ID.String(), Role: a.Role})
}

func TestRegisterLoginResolve(t *testing.T) {
	uc, _ := setup(t)
	ctx := context.Background()
	l, err := uc.Register(ctx, core.Draft{Email: " Olga@Mail.RU ", Password: "kinoshka2026", DisplayName: "Ольга", BirthDate: "2001-05-03"}, usecase.Device{UserAgent: "Firefox"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Account.Email != "olga@mail.ru" || l.Account.Role != principal.Viewer || l.Account.BirthYear() != 2001 {
		t.Fatalf("registration: %+v", l.Account)
	}
	a, _, ok, err := uc.Resolve(ctx, l.Token)
	if err != nil || !ok || a.ID != l.Account.ID {
		t.Fatalf("resolve: %v %v", ok, err)
	}
	if _, _, ok, _ := uc.Resolve(ctx, "garbage"); ok {
		t.Fatal("garbage token")
	}
	if _, err := uc.Register(ctx, core.Draft{Email: "olga@mail.ru", Password: "kinoshka2026", DisplayName: "Дубль"}, usecase.Device{}); !problem.Is(err, "email_taken") {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := uc.Register(ctx, core.Draft{Email: "kid@mail.ru", Password: "kinoshka2026", DisplayName: "Малыш", BirthDate: time.Now().AddDate(-3, 0, 0).Format("2006-01-02")}, usecase.Device{}); !problem.Is(err, "invalid") {
		t.Fatal("age below 6 rejected")
	}
	if _, err := uc.Login(ctx, "olga@mail.ru", "wrong-password-1", usecase.Device{}); !problem.Is(err, "bad_credentials") {
		t.Fatal("wrong password")
	}
	if _, err := uc.Login(ctx, "nobody@mail.ru", "kinoshka2026", usecase.Device{}); !problem.Is(err, "bad_credentials") {
		t.Fatal("same error for unknown email")
	}
}

// Сессия скользящая, но не дольше абсолютного срока; отзыв — мгновенный.
func TestSessionLifetime(t *testing.T) {
	uc, c := setup(t)
	ctx := context.Background()
	l, _ := uc.Register(ctx, core.Draft{Email: "a@b.ru", Password: "kinoshka2026", DisplayName: "Аня"}, usecase.Device{})
	for i := 0; i < 2; i++ { // активна каждые 10 дней — сессия продлевается
		c.t = c.t.Add(10 * 24 * time.Hour)
		if _, _, ok, _ := uc.Resolve(ctx, l.Token); !ok {
			t.Fatalf("sliding session expired on day %d", (i+1)*10)
		}
	}
	c.t = c.t.Add(11 * 24 * time.Hour) // 31-й день — абсолютный предел
	if _, _, ok, _ := uc.Resolve(ctx, l.Token); ok {
		t.Fatal("absolute lifetime must end the session")
	}

	l2, _ := uc.Login(ctx, "a@b.ru", "kinoshka2026", usecase.Device{UserAgent: "TV"})
	l3, _ := uc.Login(ctx, "a@b.ru", "kinoshka2026", usecase.Device{UserAgent: "Phone"})
	devs, _ := uc.Devices(as(l2.Account))
	if len(devs) != 2 {
		t.Fatalf("two live devices, got %d", len(devs))
	}
	if err := uc.RevokeDevice(as(l2.Account), l3.Session); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := uc.Resolve(ctx, l3.Token); ok {
		t.Fatal("revoked device must be logged out immediately")
	}
	stranger := core.Account{ID: uuid.New(), Role: principal.Viewer}
	if err := uc.RevokeDevice(as(stranger), l2.Session); !problem.Is(err, "not_found") {
		t.Fatal("cannot revoke someone else's session")
	}
}

func TestPasswordChangeLogsOutOtherDevices(t *testing.T) {
	uc, _ := setup(t)
	ctx := context.Background()
	l1, _ := uc.Register(ctx, core.Draft{Email: "p@b.ru", Password: "kinoshka2026", DisplayName: "Петя"}, usecase.Device{})
	l2, _ := uc.Login(ctx, "p@b.ru", "kinoshka2026", usecase.Device{})
	if err := uc.ChangePassword(as(l1.Account), l1.Session, "kinoshka2026", "p@b.ru"); !problem.Is(err, "invalid") {
		t.Fatal("password equal to email rejected")
	}
	if err := uc.ChangePassword(as(l1.Account), l1.Session, "kinoshka2026", "newpassword77"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := uc.Resolve(ctx, l1.Token); !ok {
		t.Fatal("current device stays logged in")
	}
	if _, _, ok, _ := uc.Resolve(ctx, l2.Token); ok {
		t.Fatal("other devices are logged out")
	}
}
