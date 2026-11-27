// Package core — заказы, подписки и правила доступа.
package core

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"plenka/kit/problem"
)

type Kind string

const (
	KindSubscription Kind = "subscription"
	KindPurchase     Kind = "purchase"
)

type Status string

const (
	Charging  Status = "charging"
	Failed    Status = "failed"
	Completed Status = "completed"
	Refunding Status = "refunding"
	Refunded  Status = "refunded"
)

type Plan struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PeriodDays   int    `json:"period_days"`
	PriceKopecks int64  `json:"price_kopecks"`
}

// TitleOffer — то, что billing знает о тайтле из реплики каталога.
type TitleOffer struct {
	ID             uuid.UUID
	Name           string
	Kind           string
	Status         string
	InSubscription bool
	PriceKopecks   int64
}

func (t TitleOffer) Published() bool { return t.Status == "published" }

type Order struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"-"`
	Kind        Kind       `json:"kind"`
	PlanID      string     `json:"plan_id,omitempty"`
	TitleID     *uuid.UUID `json:"title_id,omitempty"`
	TitleName   string     `json:"title_name,omitempty"`
	Amount      int64      `json:"amount_kopecks"`
	Status      Status     `json:"status"`
	Failure     string     `json:"failure,omitempty"`
	CardLast4   string     `json:"card_last4"`
	IdemKey     string     `json:"-"`
	RequestHash string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Target — ключ того, что покупается: на один ключ — один заказ в работе.
func (o Order) Target() string {
	if o.Kind == KindSubscription {
		return "subscription"
	}
	return "title:" + o.TitleID.String()
}

type Subscription struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"-"`
	PlanID            string    `json:"plan_id"`
	PlanName          string    `json:"plan_name"`
	StartedAt         time.Time `json:"started_at"`
	CurrentPeriodEnd  time.Time `json:"current_period_end"`
	CancelAtPeriodEnd bool      `json:"cancel_at_period_end"`
}

// Active — подписка действует, пока не кончился оплаченный период, даже
// если автопродление отключено.
func (s Subscription) Active(now time.Time) bool { return now.Before(s.CurrentPeriodEnd) }

type Purchase struct {
	TitleID     uuid.UUID `json:"title_id"`
	TitleName   string    `json:"title_name"`
	PurchasedAt time.Time `json:"purchased_at"`
	Amount      int64     `json:"amount_kopecks"`
}

// Card — данные карты для учебного шлюза. В системе хранятся только
// последние 4 цифры; полный номер существует лишь в памяти запроса.
type Card struct {
	Number string
}

// CleanCard — 16 цифр и корректная контрольная сумма Луна.
func CleanCard(raw string) (Card, error) {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r != ' ' && r != '-' {
			return Card{}, problem.Invalid(map[string]string{"card_number": "Номер карты — только цифры"})
		}
	}
	n := b.String()
	if len(n) != 16 || !luhn(n) {
		return Card{}, problem.Invalid(map[string]string{"card_number": "Проверьте номер карты"})
	}
	return Card{Number: n}, nil
}

func (c Card) Last4() string { return c.Number[len(c.Number)-4:] }

func luhn(s string) bool {
	sum, alt := 0, false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if alt {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// Decision — результат проверки доступа.
type Decision struct {
	Allowed    bool
	Via        string // subscription | purchase
	Denial     string // not_entitled | not_available
	ValidUntil time.Time
}

// Decide — правило доступа к просмотру. Снятый с показа тайтл недоступен
// никому, даже купившим: права на показ закончились у самого кинотеатра.
func Decide(t TitleOffer, known bool, purchased bool, sub *Subscription, now time.Time) Decision {
	if !known || !t.Published() {
		return Decision{Denial: "not_available"}
	}
	if purchased {
		return Decision{Allowed: true, Via: "purchase"}
	}
	if sub != nil && sub.Active(now) && t.InSubscription {
		return Decision{Allowed: true, Via: "subscription", ValidUntil: sub.CurrentPeriodEnd}
	}
	return Decision{Denial: "not_entitled"}
}
