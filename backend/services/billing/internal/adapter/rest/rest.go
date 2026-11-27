// Package rest — HTTP API billing.
package rest

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"plenka/billing/internal/core"
	"plenka/billing/internal/usecase"
	"plenka/kit/problem"
	"plenka/kit/web"
)

type API struct {
	Saga    *usecase.Saga
	Account *usecase.Account
}

func (a *API) Mount(r chi.Router, h func(web.Handler) http.HandlerFunc) {
	r.Get("/api/billing/plans", h(a.plans))
	r.Post("/api/billing/checkout", h(a.checkout))
	r.Get("/api/billing/orders/{id}", h(a.order))
	r.Get("/api/billing/wallet", h(a.wallet))
	r.Post("/api/billing/subscription/auto-renew", h(a.autoRenew))
	r.Get("/api/billing/offers/{title}", h(a.offer))
	r.Get("/api/billing/admin/orders", h(a.adminOrders))
	r.Get("/api/billing/admin/revenue", h(a.revenue))
	r.Post("/api/billing/admin/orders/{id}/refund", h(a.refund))
}

func (a *API) plans(w http.ResponseWriter, r *http.Request) error {
	p, err := a.Account.Plans(r.Context())
	if err != nil {
		return err
	}
	return web.JSON(w, 200, map[string]any{"items": p})
}

// checkout: 201 — оплачено и выдано; 202 — исход платежа выясняется
// (клиент опрашивает GET /orders/{id}); 402 — банк отказал; 200 — повтор.
func (a *API) checkout(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Kind       string `json:"kind"`
		PlanID     string `json:"plan_id"`
		TitleID    string `json:"title_id"`
		CardNumber string `json:"card_number"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	ci := usecase.CheckoutInput{Kind: core.Kind(in.Kind), PlanID: in.PlanID, Card: in.CardNumber, Key: r.Header.Get("Idempotency-Key")}
	if in.TitleID != "" {
		id, err := uuid.Parse(in.TitleID)
		if err != nil {
			return problem.NotFound("Тайтл")
		}
		ci.TitleID = id
	}
	o, replay, err := a.Saga.Checkout(r.Context(), ci)
	if err != nil {
		return err
	}
	status := http.StatusCreated
	switch {
	case replay:
		w.Header().Set("Idempotent-Replayed", "true")
		status = http.StatusOK
	case o.Status == core.Charging:
		w.Header().Set("Location", "/api/billing/orders/"+o.ID.String())
		status = http.StatusAccepted
	case o.Status == core.Failed:
		status = http.StatusPaymentRequired
	case o.Status == core.Refunded || o.Status == core.Refunding:
		status = http.StatusConflict
	}
	return web.JSON(w, status, o)
}

func (a *API) order(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return problem.NotFound("Заказ")
	}
	o, err := a.Saga.Order(r.Context(), id)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, o)
}

func (a *API) wallet(w http.ResponseWriter, r *http.Request) error {
	wl, err := a.Account.Wallet(r.Context())
	if err != nil {
		return err
	}
	return web.JSON(w, 200, wl)
}

func (a *API) autoRenew(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	s, err := a.Account.SetAutoRenew(r.Context(), in.Enabled)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, s)
}

func (a *API) offer(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "title"))
	if err != nil {
		return problem.NotFound("Тайтл")
	}
	o, err := a.Account.Offer(r.Context(), id)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, o)
}

func (a *API) adminOrders(w http.ResponseWriter, r *http.Request) error {
	var before *time.Time
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return problem.Invalid(map[string]string{"before": "Время в формате RFC 3339"})
		}
		before = &t
	}
	list, err := a.Account.AdminOrders(r.Context(), r.URL.Query().Get("status"), before)
	if err != nil {
		return err
	}
	next := ""
	if len(list) == 30 {
		next = list[len(list)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	type row struct {
		core.Order
		UserID string `json:"user_id"`
	}
	out := make([]row, 0, len(list))
	for _, o := range list {
		out = append(out, row{Order: o, UserID: o.UserID.String()})
	}
	return web.JSON(w, 200, map[string]any{"items": out, "next_before": next})
}

func (a *API) revenue(w http.ResponseWriter, r *http.Request) error {
	rv, err := a.Account.Revenue(r.Context())
	if err != nil {
		return err
	}
	return web.JSON(w, 200, rv)
}

// refund: 200 — заказ возвращён (refunded) или возврат в банке ещё идёт
// (refunding, recovery доведёт); 409 — заказ не оплачен.
func (a *API) refund(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return problem.NotFound("Заказ")
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := web.Bind(r, &in); err != nil {
			return err
		}
	}
	o, err := a.Saga.Refund(r.Context(), id, in.Reason)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, o)
}
