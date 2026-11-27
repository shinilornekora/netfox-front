package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"plenka/billing/internal/core"
	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

// Ledger — чтения для кабинета, проверки доступа и проекции каталога.
type Ledger interface {
	Plans(ctx context.Context) ([]core.Plan, error)
	Subscription(ctx context.Context, user uuid.UUID, now time.Time) (*core.Subscription, error)
	Purchases(ctx context.Context, user uuid.UUID) ([]core.Purchase, error)
	Orders(ctx context.Context, user uuid.UUID, limit int) ([]core.Order, error)
	SetCancelAtPeriodEnd(ctx context.Context, user uuid.UUID, now time.Time, cancel bool) (*core.Subscription, error)
	AccessFacts(ctx context.Context, user, title uuid.UUID, now time.Time) (core.TitleOffer, bool, bool, *core.Subscription, error)
	ApplyTitle(ctx context.Context, env events.Envelope, snap events.TitleSnapshot) error
	AdminOrders(ctx context.Context, status string, before *time.Time, limit int) ([]core.Order, error)
	Revenue(ctx context.Context, days int, now time.Time) (Revenue, error)
}

type Revenue struct {
	TotalKopecks        int64        `json:"total_kopecks"`
	CompletedOrders     int          `json:"completed_orders"`
	RefundedOrders      int          `json:"refunded_orders"`
	FailedOrders        int          `json:"failed_orders"`
	ActiveSubscriptions int          `json:"active_subscriptions"`
	Days                []DayRevenue `json:"days"`
}

type DayRevenue struct {
	Day     string `json:"day"`
	Kopecks int64  `json:"kopecks"`
	Orders  int    `json:"orders"`
}

type Account struct {
	Ledger Ledger
	Clock  func() time.Time
}

func (a *Account) now() time.Time {
	if a.Clock != nil {
		return a.Clock().UTC()
	}
	return time.Now().UTC()
}

func (a *Account) Plans(ctx context.Context) ([]core.Plan, error) { return a.Ledger.Plans(ctx) }

type Wallet struct {
	Subscription *core.Subscription `json:"subscription"`
	Purchases    []core.Purchase    `json:"purchases"`
	Orders       []core.Order       `json:"orders"`
}

func (a *Account) Wallet(ctx context.Context) (Wallet, error) {
	p, err := principal.Need(ctx, principal.Viewer)
	if err != nil {
		return Wallet{}, err
	}
	u := uuid.MustParse(p.UserID)
	var w Wallet
	if w.Subscription, err = a.Ledger.Subscription(ctx, u, a.now()); err != nil {
		return w, err
	}
	if w.Purchases, err = a.Ledger.Purchases(ctx, u); err != nil {
		return w, err
	}
	w.Orders, err = a.Ledger.Orders(ctx, u, 20)
	return w, err
}

// SetAutoRenew — отказ от продления: доступ сохраняется до конца
// оплаченного периода, деньги не возвращаются.
func (a *Account) SetAutoRenew(ctx context.Context, renew bool) (*core.Subscription, error) {
	p, err := principal.Need(ctx, principal.Viewer)
	if err != nil {
		return nil, err
	}
	s, err := a.Ledger.SetCancelAtPeriodEnd(ctx, uuid.MustParse(p.UserID), a.now(), !renew)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, problem.NotFound("Активная подписка")
	}
	return s, nil
}

// Offer — что зритель может сделать с тайтлом (для кнопок на странице).
type Offer struct {
	Known              bool       `json:"known"`
	Owned              bool       `json:"owned"`
	SubscriptionActive bool       `json:"subscription_active"`
	InSubscription     bool       `json:"in_subscription"`
	PriceKopecks       int64      `json:"price_kopecks"`
	CanWatch           bool       `json:"can_watch"`
	Via                string     `json:"via,omitempty"`
	ValidUntil         *time.Time `json:"valid_until,omitempty"`
}

func (a *Account) Offer(ctx context.Context, title uuid.UUID) (Offer, error) {
	p, err := principal.Need(ctx)
	if err != nil {
		return Offer{}, err
	}
	d, t, known, owned, sub, err := a.Check(ctx, uuid.MustParse(p.UserID), title)
	if err != nil {
		return Offer{}, err
	}
	o := Offer{Known: known, Owned: owned, SubscriptionActive: sub != nil && sub.Active(a.now()),
		InSubscription: t.InSubscription, PriceKopecks: t.PriceKopecks, CanWatch: d.Allowed, Via: d.Via}
	if !d.ValidUntil.IsZero() {
		v := d.ValidUntil
		o.ValidUntil = &v
	}
	return o, nil
}

// Check — решение о доступе (gRPC Access.Check и Offer).
func (a *Account) Check(ctx context.Context, user, title uuid.UUID) (core.Decision, core.TitleOffer, bool, bool, *core.Subscription, error) {
	t, known, owned, sub, err := a.Ledger.AccessFacts(ctx, user, title, a.now())
	if err != nil {
		return core.Decision{}, t, false, false, nil, err
	}
	return core.Decide(t, known, owned, sub, a.now()), t, known, owned, sub, nil
}

// HandleCatalog — проекция событий каталога.
func (a *Account) HandleCatalog(ctx context.Context, env events.Envelope) error {
	snap, err := bus.Decode[events.TitleSnapshot](env)
	if err != nil {
		return err
	}
	return a.Ledger.ApplyTitle(ctx, env, snap)
}

func (a *Account) AdminOrders(ctx context.Context, status string, before *time.Time) ([]core.Order, error) {
	if _, err := principal.Need(ctx, principal.Admin); err != nil {
		return nil, err
	}
	return a.Ledger.AdminOrders(ctx, status, before, 30)
}

func (a *Account) Revenue(ctx context.Context) (Revenue, error) {
	if _, err := principal.Need(ctx, principal.Admin); err != nil {
		return Revenue{}, err
	}
	return a.Ledger.Revenue(ctx, 30, a.now())
}
