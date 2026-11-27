// Package bus — Kafka через franz-go.
package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"plenka/contracts/events"
	"plenka/kit/outbox"
)

type Producer struct{ cl *kgo.Client }

func NewProducer(brokers []string) (*Producer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(10*time.Millisecond),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, err
	}
	return &Producer{cl: cl}, nil
}

func (p *Producer) Publish(ctx context.Context, msgs []outbox.Message) error {
	recs := make([]*kgo.Record, 0, len(msgs))
	for _, m := range msgs {
		recs = append(recs, &kgo.Record{Topic: m.Topic, Key: []byte(m.Key), Value: m.Value})
	}
	return p.cl.ProduceSync(ctx, recs...).FirstErr()
}

func (p *Producer) Close() { p.cl.Close() }

// EnsureTopics создаёт топики (и их .dlq) заранее, повторяя, пока брокер
// не поднимется.
func EnsureTopics(ctx context.Context, brokers []string, log *slog.Logger, partitions int32, topics ...string) {
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		log.Error("kafka admin", "err", err)
		return
	}
	defer cl.Close()
	adm := kadm.NewClient(cl)
	var all []string
	for _, t := range topics {
		all = append(all, t, t+".dlq")
	}
	for i := 1; i <= 40 && ctx.Err() == nil; i++ {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := adm.CreateTopics(cctx, partitions, 1, nil, all...)
		cancel()
		if err == nil {
			ok := true
			for _, r := range resp {
				if r.Err != nil && !errors.Is(r.Err, kerr.TopicAlreadyExists) {
					ok = false
					err = r.Err
				}
			}
			if ok {
				return
			}
		}
		log.Warn("waiting for kafka", "attempt", i, "err", err)
		time.Sleep(2 * time.Second)
	}
}

// Permanent — ошибка, которую бесполезно повторять.
var Permanent = errors.New("permanent")

// IsPermanent — повтор того же события даст ту же ошибку: явный Permanent
// или ошибка данных PostgreSQL — класс 22 (неверное значение: не UUID,
// переполнение), 23 (нарушение ограничения) и отказ политики RLS (42501 с
// «row-level security»: событие пытается записать строку не тому зрителю).
// Остальное (связь, тайм-аут, 40001, нет прав на таблицу) — временное: его
// повторяют, потому что это чинится настройкой, а не правкой события.
func IsPermanent(err error) bool {
	if errors.Is(err, Permanent) {
		return true
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		// Отказ RLS отличаем от «нет прав на таблицу» по функции сервера, а не
		// по тексту: текст переводится (lc_messages), имя функции — нет.
		// Класс 23 целиком постоянный: и 23505 при сохранённом порядке внутри
		// партиции значит, что событие противоречит уже записанному, а не
		// что «повезёт в другой раз».
		return strings.HasPrefix(pe.Code, "22") || strings.HasPrefix(pe.Code, "23") ||
			(pe.Code == "42501" && pe.Routine == "ExecWithCheckOptions")
	}
	return false
}

type Handler func(ctx context.Context, env events.Envelope) error

// Delivery — откуда пришла запись: топик, раздел и смещение. Обработчик
// может достать их из контекста (DeliveryFrom) — для журнала и для тестов,
// которые проверяют, что после сбоя читается то же смещение, а не новое.
type Delivery struct {
	Topic     string
	Partition int32
	Offset    int64
}

type deliveryKey struct{}

func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}

type Consumer struct {
	Brokers []string
	Group   string
	Topics  []string
	Handle  Handler
	DLQ     *Producer
	Log     *slog.Logger
	// Attempts — после стольких неудачных попыток временная ошибка
	// становится тревогой в логе (уровень ERROR). Повторы продолжаются.
	Attempts int
	// Timeout — жёсткий предел одной попытки обработки (по умолчанию 10 с):
	// зависшая база или сосед не держат партицию бесконечно (РХД ПР-5).
	Timeout time.Duration
	// MaxBackoff — потолок паузы между повторами (по умолчанию 5 с).
	MaxBackoff time.Duration
}

// Run: at-least-once. Смещение фиксируется вручную и только для записей,
// судьба которых решена: обработана или положена в DLQ.
//
//   - Постоянная ошибка (bus.Permanent, битый конверт) — в DLQ, дальше.
//   - Временная (база недоступна, тайм-аут) — повтор с паузой, пока не
//     получится. Партиция стоит: порядок событий агрегата важнее, а
//     терять корректное событие нельзя. Цена — одно «зависшее» событие
//     задерживает остальные (TRADEOFFS 4.18).
//   - DLQ недоступна — публикация в DLQ тоже повторяется; смещение ядовитой
//     записи не фиксируется, пока она не легла в DLQ.
//   - Остановка процесса посреди записи — её смещение не фиксируется, после
//     перезапуска запись придёт снова, повтор отсечёт inbox получателя.
func (c *Consumer) Run(ctx context.Context) error {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(c.Brokers...),
		kgo.ConsumerGroup(c.Group),
		kgo.ConsumeTopics(c.Topics...),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return err
	}
	defer cl.Close()
	for {
		fetches := cl.PollRecords(ctx, 50)
		if ctx.Err() != nil {
			cl.AllowRebalance()
			return nil
		}
		fetches.EachError(func(t string, p int32, err error) {
			c.Log.Warn("kafka fetch", "topic", t, "partition", p, "err", err)
		})
		var done []*kgo.Record
		stopped := false
		for it := fetches.RecordIter(); !it.Done(); {
			rec := it.Next()
			if !c.process(ctx, rec) {
				stopped = true // остановка: эта и следующие записи не фиксируются
				break
			}
			done = append(done, rec)
		}
		if len(done) > 0 {
			// Фиксируем решённое даже при остановке — иначе после рестарта
			// придёт лишнее (inbox отсечёт, но это лишняя работа).
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			if err := cl.CommitRecords(cctx, done...); err != nil {
				c.Log.Warn("kafka commit", "err", err)
			}
			cancel()
		}
		cl.AllowRebalance()
		if stopped {
			return nil
		}
	}
}

// process возвращает true, если судьба записи решена и её можно фиксировать.
func (c *Consumer) process(ctx context.Context, rec *kgo.Record) bool {
	var env events.Envelope
	if err := json.Unmarshal(rec.Value, &env); err != nil || env.ID == "" {
		return c.dead(ctx, rec, fmt.Errorf("%w: bad envelope: %v", Permanent, err))
	}
	dctx := context.WithValue(ctx, deliveryKey{}, Delivery{Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset})
	for i := 1; ; i++ {
		hctx, cancel := context.WithTimeout(dctx, c.timeout())
		err := c.Handle(hctx, env)
		cancel()
		if err == nil {
			return true
		}
		if IsPermanent(err) {
			return c.dead(ctx, rec, err)
		}
		if ctx.Err() != nil {
			return false
		}
		if i == c.attempts() {
			c.Log.Error("event stuck, still retrying", "type", env.Type, "id", env.ID, "attempt", i, "err", err)
		} else {
			c.Log.Warn("event retry", "type", env.Type, "id", env.ID, "attempt", i, "err", err)
		}
		if !c.pause(ctx, i) {
			return false
		}
	}
}

// dead кладёт запись в DLQ, повторяя публикацию, пока она не удастся.
func (c *Consumer) dead(ctx context.Context, rec *kgo.Record, cause error) bool {
	c.Log.Error("event to DLQ", "topic", rec.Topic, "offset", rec.Offset, "err", cause)
	if c.DLQ == nil {
		return true
	}
	r := &kgo.Record{Topic: rec.Topic + ".dlq", Key: rec.Key, Value: rec.Value, Headers: []kgo.RecordHeader{
		{Key: "reason", Value: []byte(cause.Error())},
		{Key: "group", Value: []byte(c.Group)},
		{Key: "source_offset", Value: []byte(fmt.Sprint(rec.Offset))},
	}}
	for i := 1; ; i++ {
		pctx, cancel := context.WithTimeout(ctx, c.timeout())
		err := c.DLQ.cl.ProduceSync(pctx, r).FirstErr()
		cancel()
		if err == nil {
			return true
		}
		c.Log.Error("dlq publish failed, offset not committed", "attempt", i, "err", err)
		if !c.pause(ctx, i) {
			return false
		}
	}
}

func (c *Consumer) pause(ctx context.Context, attempt int) bool {
	d := time.Duration(attempt*attempt) * 150 * time.Millisecond
	limit := c.MaxBackoff
	if limit <= 0 {
		limit = 5 * time.Second
	}
	if d > limit {
		d = limit
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (c *Consumer) attempts() int {
	if c.Attempts > 0 {
		return c.Attempts
	}
	return 4
}

func (c *Consumer) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 10 * time.Second
}

// Decode разбирает полезную нагрузку; ошибка формата — постоянная.
func Decode[T any](env events.Envelope) (T, error) {
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		return v, fmt.Errorf("%w: %s: %v", Permanent, env.Type, err)
	}
	return v, nil
}
