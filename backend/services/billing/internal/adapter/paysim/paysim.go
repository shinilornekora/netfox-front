// Package paysim — учебный платёжный шлюз (имитация внешнего банка).
//
// Тестовые карты (как у реальных шлюзов):
//
//	4242 4242 4242 4242 — успешная оплата
//	4000 0000 0000 0002 — отказ банка
//	4000 0000 0000 9995 — недостаточно средств
//	4000 0000 0000 0259 — деньги списываются, но ответ «теряется» (тайм-аут)
//
// Charge идемпотентен по ref (id заказа): повторный вызов возвращает уже
// записанный результат и не списывает деньги второй раз.
package paysim

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/billing/internal/core"
	"plenka/billing/internal/usecase"
)

type Gateway struct{ DB *pgxpool.Pool }

var _ usecase.Provider = Gateway{}

const lostResponse = "4000000000000259"

func outcome(card core.Card) (status, reason string) {
	switch card.Number {
	case "4000000000000002":
		return "declined", "Банк отклонил операцию"
	case "4000000000009995":
		return "declined", "Недостаточно средств на карте"
	default:
		return "succeeded", ""
	}
}

func (g Gateway) Charge(ctx context.Context, ref string, amount int64, card core.Card) (usecase.ChargeResult, error) {
	status, reason := outcome(card)
	// Запись в «банке» делается независимо от судьбы запроса клиента —
	// поэтому context.WithoutCancel: банк не узнаёт, что клиент перестал ждать.
	bctx := context.WithoutCancel(ctx)
	if _, err := g.DB.Exec(bctx, `INSERT INTO paysim_charges(ref, amount_kopecks, card_last4, status, decline_reason)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (ref) DO NOTHING`, ref, amount, card.Last4(), status, reason); err != nil {
		return usecase.ChargeResult{}, err
	}
	if card.Number == lostResponse {
		<-ctx.Done() // ответ банка не приходит, клиент упирается в тайм-аут
		return usecase.ChargeResult{}, ctx.Err()
	}
	if err := g.DB.QueryRow(bctx, `SELECT status, decline_reason FROM paysim_charges WHERE ref=$1`, ref).Scan(&status, &reason); err != nil {
		return usecase.ChargeResult{}, err
	}
	return usecase.ChargeResult{Succeeded: status == "succeeded", Reason: reason}, nil
}

func (g Gateway) Refund(ctx context.Context, ref string) error {
	tag, err := g.DB.Exec(ctx, `UPDATE paysim_charges SET status='refunded', refunded_at=now() WHERE ref=$1 AND status IN ('succeeded','refunded')`, ref)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("paysim: nothing to refund")
	}
	return err
}

func (g Gateway) Lookup(ctx context.Context, ref string) (string, bool, error) {
	var st string
	err := g.DB.QueryRow(ctx, `SELECT status FROM paysim_charges WHERE ref=$1`, ref).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return st, err == nil, err
}
