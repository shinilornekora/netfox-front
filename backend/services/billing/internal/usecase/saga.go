// Package usecase — оркестратор оплаты и правила доступа.
//
// Оплата — оркестрируемая сага из трёх шагов с явным состоянием в orders:
//
//  1. prepare (SERIALIZABLE): проверить, что покупать можно, и записать
//     заказ в статусе charging;
//  2. charge (вне транзакции): списать деньги во внешнем шлюзе;
//  3. fulfil (SERIALIZABLE): выдать доступ и закрыть заказ.
//
// Компенсация: если деньги списаны, а выдать доступ нельзя (за это время
// доступ появился другим путём), оркестратор возвращает деньги
// (refunding → refunded). Если ответ шлюза потерян (тайм-аут), заказ
// остаётся в charging, и recovery-воркер выясняет у шлюза, чем всё
// кончилось. Оркестратор — единственный, кто знает, где сейчас процесс.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"plenka/billing/internal/core"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

// ChargeResult — ответ шлюза, если он вообще ответил.
type ChargeResult struct {
	Succeeded bool
	Reason    string // причина отказа
}

// Provider — порт платёжного шлюза. Ошибка Charge означает «исход
// неизвестен» (тайм-аут, обрыв): деньги могли списаться.
type Provider interface {
	Charge(ctx context.Context, ref string, amount int64, card core.Card) (ChargeResult, error)
	Refund(ctx context.Context, ref string) error
	Lookup(ctx context.Context, ref string) (status string, found bool, err error)
}

// PrepTx — чтения и запись шага prepare в одной SERIALIZABLE-транзакции.
type PrepTx interface {
	OrderByKey(ctx context.Context, user uuid.UUID, key string) (*core.Order, error)
	Plan(ctx context.Context, id string) (core.Plan, error)
	Title(ctx context.Context, id uuid.UUID) (core.TitleOffer, bool, error)
	HasPurchase(ctx context.Context, user, title uuid.UUID) (bool, error)
	ActiveSubscription(ctx context.Context, user uuid.UUID, now time.Time) (*core.Subscription, error)
	InFlight(ctx context.Context, user uuid.UUID, target string) (bool, error)
	InsertOrder(ctx context.Context, o core.Order) error
}

type Store interface {
	Prepare(ctx context.Context, fn func(PrepTx) error) error
	// Fulfil выдаёт доступ и переводит заказ в completed. Ошибка с кодом
	// "cannot_fulfil" — выдать нельзя, нужна компенсация.
	Fulfil(ctx context.Context, orderID uuid.UUID, now time.Time) (core.Order, error)
	// Transition меняет статус, только если текущий — один из from.
	Transition(ctx context.Context, id uuid.UUID, from []core.Status, to core.Status, failure string) (core.Order, error)
	// Revoke забирает выданное право и переводит completed → refunding.
	Revoke(ctx context.Context, id uuid.UUID, reason string, now time.Time) (core.Order, error)
	Order(ctx context.Context, id uuid.UUID) (core.Order, error)
	Stuck(ctx context.Context, olderThan time.Time) ([]core.Order, error)
}

type Saga struct {
	Store         Store
	Provider      Provider
	Log           *slog.Logger
	ChargeTimeout time.Duration
	Clock         func() time.Time
}

func (s *Saga) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

type CheckoutInput struct {
	Kind    core.Kind
	PlanID  string
	TitleID uuid.UUID
	Card    string
	Key     string // Idempotency-Key
}

func (in CheckoutInput) hash() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", in.Kind, in.PlanID, in.TitleID)))
	return hex.EncodeToString(sum[:])
}

// Checkout — точка входа саги. Возвращает заказ в одном из статусов:
// completed (всё хорошо), failed (отказ банка), charging (исход платежа
// неизвестен, ждём сверки), refunded (деньги вернули).
func (s *Saga) Checkout(ctx context.Context, in CheckoutInput) (core.Order, bool, error) {
	p, err := principal.Need(ctx, principal.Viewer)
	if err != nil {
		return core.Order{}, false, err
	}
	user := uuid.MustParse(p.UserID)
	if l := len(in.Key); l < 8 || l > 100 {
		return core.Order{}, false, problem.Invalid(map[string]string{"idempotency_key": "Нужен заголовок Idempotency-Key"})
	}
	card, err := core.CleanCard(in.Card)
	if err != nil {
		return core.Order{}, false, err
	}

	// ---- шаг 1: prepare ----
	var order core.Order
	replay := false
	err = s.Store.Prepare(ctx, func(tx PrepTx) error {
		replay = false
		if prev, err := tx.OrderByKey(ctx, user, in.Key); err != nil {
			return err
		} else if prev != nil {
			if prev.RequestHash != in.hash() {
				return problem.Conflict("idempotency_mismatch", "Этот ключ уже использован для другой покупки")
			}
			order, replay = *prev, true
			return nil
		}
		o := core.Order{ID: uuid.New(), UserID: user, Kind: in.Kind, Status: core.Charging, CardLast4: card.Last4(),
			IdemKey: in.Key, RequestHash: in.hash(), CreatedAt: s.now(), UpdatedAt: s.now()}
		switch in.Kind {
		case core.KindSubscription:
			plan, err := tx.Plan(ctx, in.PlanID)
			if err != nil {
				return err
			}
			sub, err := tx.ActiveSubscription(ctx, user, s.now())
			if err != nil {
				return err
			}
			if sub != nil {
				return problem.Conflict("already_subscribed", "Подписка уже действует до "+sub.CurrentPeriodEnd.Format("02.01.2006"))
			}
			o.PlanID, o.Amount = plan.ID, plan.PriceKopecks
		case core.KindPurchase:
			t, known, err := tx.Title(ctx, in.TitleID)
			if err != nil {
				return err
			}
			if !known || !t.Published() || t.PriceKopecks == 0 {
				return problem.Conflict("not_for_sale", "Этот фильм сейчас не продаётся")
			}
			owned, err := tx.HasPurchase(ctx, user, t.ID)
			if err != nil {
				return err
			}
			if owned {
				return problem.Conflict("already_owned", "Фильм уже куплен — он есть в разделе «Покупки»")
			}
			tid := t.ID
			o.TitleID, o.TitleName, o.Amount = &tid, t.Name, t.PriceKopecks
		default:
			return problem.Invalid(map[string]string{"kind": "subscription или purchase"})
		}
		// Инвариант «не больше одного заказа в работе на одну цель» связывает
		// несколько строк: проверка + вставка. Две параллельные транзакции
		// обе увидели бы «нет» и обе вставили бы (write skew). SERIALIZABLE
		// замечает это и откатывает одну из них; повтор уже видит первый заказ.
		busy, err := tx.InFlight(ctx, user, o.Target())
		if err != nil {
			return err
		}
		if busy {
			return problem.Conflict("order_in_progress", "Предыдущая оплата ещё обрабатывается — подождите несколько секунд")
		}
		order = o
		return tx.InsertOrder(ctx, o)
	})
	if err != nil {
		return core.Order{}, false, err
	}
	if replay {
		// Повтор того же запроса: если заказ застрял, не трогаем — его
		// доведёт recovery. Возвращаем текущее состояние.
		cur, err := s.Store.Order(ctx, order.ID)
		return cur, true, err
	}

	// ---- шаг 2: charge ----
	cctx, cancel := context.WithTimeout(ctx, s.chargeTimeout())
	res, err := s.Provider.Charge(cctx, order.ID.String(), order.Amount, card)
	cancel()
	if err != nil {
		// Исход неизвестен. Нельзя ни выдавать доступ, ни объявлять отказ:
		// деньги могли списаться. Заказ остаётся charging до сверки.
		s.Log.Warn("charge outcome unknown", "order", order.ID, "err", err)
		return order, false, nil
	}
	if !res.Succeeded {
		o, err := s.Store.Transition(ctx, order.ID, []core.Status{core.Charging}, core.Failed, res.Reason)
		return o, false, err
	}

	// ---- шаг 3: fulfil (+ компенсация) ----
	o, err := s.fulfil(ctx, order.ID)
	return o, false, err
}

func (s *Saga) chargeTimeout() time.Duration {
	if s.ChargeTimeout > 0 {
		return s.ChargeTimeout
	}
	return 3 * time.Second
}

func (s *Saga) fulfil(ctx context.Context, id uuid.UUID) (core.Order, error) {
	o, err := s.Store.Fulfil(ctx, id, s.now())
	if err == nil {
		return o, nil
	}
	if !problem.Is(err, "cannot_fulfil") {
		return o, err // временная ошибка: заказ в charging, recovery повторит
	}
	return s.compensate(ctx, id, problem.From(err).Title)
}

// compensate — возврат денег. Сначала фиксируем намерение (refunding), потом
// идём в шлюз: если процесс упадёт между шагами, recovery увидит refunding и
// повторит возврат (Refund у шлюза идемпотентен).
func (s *Saga) compensate(ctx context.Context, id uuid.UUID, why string) (core.Order, error) {
	if _, err := s.Store.Transition(ctx, id, []core.Status{core.Charging}, core.Refunding, why); err != nil {
		return core.Order{}, err
	}
	if err := s.Provider.Refund(ctx, id.String()); err != nil {
		s.Log.Warn("refund failed, will retry", "order", id, "err", err)
		return s.Store.Order(ctx, id)
	}
	return s.Store.Transition(ctx, id, []core.Status{core.Refunding}, core.Refunded, why)
}

// Refund — возврат оплаченного заказа администратором (жалоба зрителя,
// ошибка цены). Тот же порядок, что у компенсации: сначала в базе
// зафиксировать намерение и забрать право, потом идти в банк.
func (s *Saga) Refund(ctx context.Context, id uuid.UUID, reason string) (core.Order, error) {
	if _, err := principal.Need(ctx, principal.Admin); err != nil {
		return core.Order{}, err
	}
	if reason == "" {
		reason = "Возврат по решению администратора"
	}
	o, err := s.Store.Revoke(ctx, id, reason, s.now())
	if err != nil || o.Status == core.Refunded {
		return o, err
	}
	if err := s.Provider.Refund(ctx, id.String()); err != nil {
		s.Log.Warn("refund failed, recovery will retry", "order", id, "err", err)
		return o, nil
	}
	return s.Store.Transition(ctx, id, []core.Status{core.Refunding}, core.Refunded, reason)
}

// Recover доводит до конца заказы, застрявшие дольше grace: сверяется со
// шлюзом и продолжает сагу с нужного шага.
func (s *Saga) Recover(ctx context.Context, grace time.Duration) (int, error) {
	stuck, err := s.Store.Stuck(ctx, s.now().Add(-grace))
	if err != nil {
		return 0, err
	}
	done := 0
	for _, o := range stuck {
		var err error
		switch o.Status {
		case core.Charging:
			status, found, lerr := s.Provider.Lookup(ctx, o.ID.String())
			switch {
			case lerr != nil:
				err = lerr
			case !found:
				// Шлюз о платеже не знает — деньги не списывались.
				_, err = s.Store.Transition(ctx, o.ID, []core.Status{core.Charging}, core.Failed, "Платёж не дошёл до банка")
			case status == "succeeded":
				_, err = s.fulfil(ctx, o.ID)
			case status == "refunded":
				_, err = s.Store.Transition(ctx, o.ID, []core.Status{core.Charging}, core.Refunded, "Платёж отменён банком")
			default:
				_, err = s.Store.Transition(ctx, o.ID, []core.Status{core.Charging}, core.Failed, "Банк отклонил платёж")
			}
		case core.Refunding:
			if err = s.Provider.Refund(ctx, o.ID.String()); err == nil {
				_, err = s.Store.Transition(ctx, o.ID, []core.Status{core.Refunding}, core.Refunded, o.Failure)
			}
		}
		if err != nil {
			s.Log.Warn("recovery step failed", "order", o.ID, "status", o.Status, "err", err)
			continue
		}
		done++
	}
	return done, nil
}

// RunRecovery — фоновый цикл сверки.
func (s *Saga) RunRecovery(ctx context.Context, every, grace time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.Recover(ctx, grace); err != nil {
				s.Log.Warn("recovery", "err", err)
			} else if n > 0 {
				s.Log.Info("orders recovered", "count", n)
			}
		}
	}
}

// Order — заказ зрителя (чужой — как не найденный).
func (s *Saga) Order(ctx context.Context, id uuid.UUID) (core.Order, error) {
	p, err := principal.Need(ctx)
	if err != nil {
		return core.Order{}, err
	}
	o, err := s.Store.Order(ctx, id)
	if err != nil {
		return o, err
	}
	if o.UserID.String() != p.UserID && p.Role != principal.Admin {
		return core.Order{}, problem.NotFound("Заказ")
	}
	return o, nil
}

var ErrNotFound = errors.New("not found")
