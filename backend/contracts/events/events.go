// Package events — общий контракт событий между сервисами.
//
// В отличие от «tolerant reader», где каждый потребитель объявляет свои
// структуры, здесь контракт один и версионируется вместе с .proto в модуле
// contracts. Производитель и потребители компилируются против одной
// структуры: переименование поля ломает сборку, а не продакшен.
// Цена — модуль contracts становится общей зависимостью всех сервисов.
package events

import "time"

const TopicTitles = "catalog.titles.v1"

// Типы событий тайтла.
const (
	TitlePublished = "title.published"
	TitleUpdated   = "title.updated"
	TitleWithdrawn = "title.withdrawn"
)

// Envelope — конверт любого события.
type Envelope struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Version     int       `json:"schema_version"`
	Aggregate   string    `json:"aggregate_id"`
	Revision    int64     `json:"revision"` // версия агрегата: потребитель игнорирует старые
	OccurredAt  time.Time `json:"occurred_at"`
	Source      string    `json:"source"`
	Correlation string    `json:"correlation_id,omitempty"`
	Data        []byte    `json:"data"` // JSON полезной нагрузки
}

// Episode — серия в составе снимка сериала.
type Episode struct {
	ID          string `json:"id"`
	Season      int    `json:"season"`
	Number      int    `json:"number"`
	Name        string `json:"name"`
	DurationSec int    `json:"duration_sec"`
	Media       string `json:"media,omitempty"` // имя видеофайла в хранилище плеера
}

// TitleSnapshot — полное публичное состояние тайтла.
type TitleSnapshot struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"` // film | series
	Name           string    `json:"name"`
	Year           int       `json:"year"`
	AgeRating      int       `json:"age_rating"` // 0, 6, 12, 16, 18
	Status         string    `json:"status"`     // published | withdrawn
	InSubscription bool      `json:"in_subscription"`
	PriceKopecks   int64     `json:"price_kopecks"` // 0 — не продаётся отдельно
	DurationSec    int       `json:"duration_sec"`  // для фильма
	Episodes       []Episode `json:"episodes"`      // для сериала
	PosterHue      int       `json:"poster_hue"`
	PosterRev      int64     `json:"poster_rev,omitempty"` // 0 — постера нет
	Media          string    `json:"media,omitempty"`      // видеофайл фильма; у сериала — у каждой серии
	Revision       int64     `json:"revision"`
}

// ---------- billing.orders.v1 ----------

// TopicOrders — изменения заказов billing (РХД ПР-4/5). Полное описание полей
// и алгоритма потребителя — docs/evidence/04-outbox.md.
const TopicOrders = "billing.orders.v1"

// OrderStatusChanged — заказ перешёл в новый статус.
const OrderStatusChanged = "order.status_changed"

// OrderSnapshot — состояние заказа после перехода. Envelope.Aggregate = id
// заказа, Envelope.Revision = OrderRevision(status): номер шага саги,
// монотонный вдоль любых допустимых переходов.
type OrderSnapshot struct {
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`               // purchase | subscription
	TitleID   string    `json:"title_id,omitempty"` // для покупки
	PlanID    string    `json:"plan_id,omitempty"`  // для подписки
	AmountKop int64     `json:"amount_kopecks"`
	Status    string    `json:"status"` // charging | completed | failed | refunding | refunded
	ChangedAt time.Time `json:"changed_at"`
}

// OrderRevision — номер шага саги по статусу: charging 1 → completed/failed 2
// → refunding 3 → refunded 4. Допустимые переходы его только увеличивают,
// поэтому потребитель может отбрасывать события со старой ревизией.
func OrderRevision(status string) int64 {
	switch status {
	case "charging":
		return 1
	case "completed", "failed":
		return 2
	case "refunding":
		return 3
	case "refunded":
		return 4
	}
	return 0
}
