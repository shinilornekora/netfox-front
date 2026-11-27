package bus

// РХД ПР-5 на настоящей Kafka: poison pill уходит в DLQ, временная ошибка
// не теряет событие, падение после коммита в базу приводит к повторной
// доставке, которую отсекает inbox.
//
//	TEST_KAFKA_BROKERS=localhost:9092 go test ./bus/
//
// Тест падения после коммита использует ещё и настоящий PostgreSQL
// (TEST_PG_DSN), чтобы inbox был таблицей, а не словарём в памяти.
//
// Без переменной тест пропускается.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twmb/franz-go/pkg/kgo"

	"plenka/contracts/events"
	"plenka/kit/outbox"
	"plenka/kit/pgtest"
)

func TestPoisonPillGoesToDLQ(t *testing.T) {
	brokers := os.Getenv("TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("TEST_KAFKA_BROKERS не задан")
	}
	bs := strings.Split(brokers, ",")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	topic := "test.poison." + uuid.NewString()[:8]
	EnsureTopics(ctx, bs, log, 1, topic)

	prod, err := NewProducer(bs)
	if err != nil {
		t.Fatal(err)
	}
	defer prod.Close()
	good := func(id string) []byte {
		return []byte(`{"id":"` + id + `","type":"x","schema_version":1,"aggregate_id":"a","revision":1,"data":"e30="}`)
	}
	g1, g2 := uuid.NewString(), uuid.NewString()
	recs := []*kgo.Record{
		{Topic: topic, Key: []byte("a"), Value: good(g1)},
		{Topic: topic, Key: []byte("a"), Value: []byte("это не JSON")}, // poison pill
		{Topic: topic, Key: []byte("a"), Value: good(g2)},
	}
	if err := prod.cl.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var handled []string
	run := func(d time.Duration) {
		c := &Consumer{Brokers: bs, Group: "test-" + topic, Topics: []string{topic}, DLQ: prod, Log: log, Attempts: 2,
			Timeout: 2 * time.Second, Handle: func(_ context.Context, env events.Envelope) error {
				mu.Lock()
				handled = append(handled, env.ID)
				mu.Unlock()
				return nil
			}}
		cctx, ccancel := context.WithTimeout(ctx, d)
		defer ccancel()
		_ = c.Run(cctx)
	}
	run(8 * time.Second)
	if len(handled) != 2 || handled[0] != g1 || handled[1] != g2 {
		t.Fatalf("good events around the poison pill must be handled in order: %v", handled)
	}

	// Ядовитое сообщение лежит в DLQ с причиной.
	dl, err := kgo.NewClient(kgo.SeedBrokers(bs...), kgo.ConsumeTopics(topic+".dlq"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Close()
	fctx, fcancel := context.WithTimeout(ctx, 10*time.Second)
	defer fcancel()
	var got *kgo.Record
	for got == nil && fctx.Err() == nil {
		dl.PollFetches(fctx).EachRecord(func(r *kgo.Record) { got = r })
	}
	if got == nil || string(got.Value) != "это не JSON" {
		t.Fatalf("poison pill not in DLQ: %+v", got)
	}
	var reason string
	for _, h := range got.Headers {
		if h.Key == "reason" {
			reason = string(h.Value)
		}
	}
	if !strings.Contains(reason, "bad envelope") {
		t.Fatalf("DLQ reason: %q", reason)
	}

	// Смещения зафиксированы: тот же consumer group после перезапуска ничего
	// не получает повторно — ни хорошие события, ни яд.
	mu.Lock()
	handled = nil
	mu.Unlock()
	run(6 * time.Second)
	if len(handled) != 0 {
		t.Fatalf("redelivered after restart: %v", handled)
	}
}

type kafkaEnv struct {
	bs    []string
	prod  *Producer
	log   *slog.Logger
	topic string
}

func newKafka(t *testing.T, ctx context.Context, name string) *kafkaEnv {
	t.Helper()
	brokers := os.Getenv("TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("TEST_KAFKA_BROKERS не задан")
	}
	k := &kafkaEnv{bs: strings.Split(brokers, ","), log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		topic: "test." + name + "." + uuid.NewString()[:8]}
	EnsureTopics(ctx, k.bs, k.log, 1, k.topic)
	prod, err := NewProducer(k.bs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(prod.Close)
	k.prod = prod
	return k
}

func envelope(id string) []byte {
	return []byte(`{"id":"` + id + `","type":"x","schema_version":1,"aggregate_id":"a","revision":1,"data":"e30="}`)
}

func (k *kafkaEnv) send(t *testing.T, ctx context.Context, ids ...string) {
	t.Helper()
	var recs []*kgo.Record
	for _, id := range ids {
		recs = append(recs, &kgo.Record{Topic: k.topic, Key: []byte("a"), Value: envelope(id)})
	}
	if err := k.prod.cl.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

// run запускает потребителя группы до истечения d или до остановки изнутри.
func (k *kafkaEnv) run(ctx context.Context, d time.Duration, h func(ctx context.Context, env events.Envelope, stop func()) error) {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	c := &Consumer{Brokers: k.bs, Group: "test-" + k.topic, Topics: []string{k.topic}, DLQ: k.prod, Log: k.log,
		Attempts: 2, Timeout: 2 * time.Second, MaxBackoff: 300 * time.Millisecond,
		Handle: func(hctx context.Context, env events.Envelope) error { return h(hctx, env, cancel) }}
	_ = c.Run(cctx)
}

func (k *kafkaEnv) dlqCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	dl, err := kgo.NewClient(kgo.SeedBrokers(k.bs...), kgo.ConsumeTopics(k.topic+".dlq"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Close()
	fctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	n := 0
	for fctx.Err() == nil {
		dl.PollFetches(fctx).EachRecord(func(*kgo.Record) { n++ })
	}
	return n
}

// Временная ошибка (база легла на несколько секунд) — событие не уходит в
// DLQ и не теряется: потребитель повторяет, пока не получится, и только
// потом берётся за следующее.
func TestTransientErrorIsRetriedNotDropped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "transient")
	g1, g2 := uuid.NewString(), uuid.NewString()
	k.send(t, ctx, g1, g2)
	var mu sync.Mutex
	var calls, handled []string
	k.run(ctx, 15*time.Second, func(_ context.Context, env events.Envelope, stop func()) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, env.ID)
		if env.ID == g1 && len(calls) <= 5 { // больше, чем Attempts: раньше тут был DLQ
			return errors.New("connection refused")
		}
		handled = append(handled, env.ID)
		if len(handled) == 2 {
			stop()
		}
		return nil
	})
	if len(handled) != 2 || handled[0] != g1 || handled[1] != g2 {
		t.Fatalf("transient failure must delay, not drop or reorder: handled=%v calls=%d", handled, len(calls))
	}
	if n := k.dlqCount(t, ctx); n != 0 {
		t.Fatalf("transient error sent %d records to DLQ", n)
	}
}

// Падение после коммита в базу: обработчик в одной транзакции настоящего
// PostgreSQL отметил событие в inbox и записал эффект, после чего процесс
// остановлен до фиксации смещения (отмена контекста: запись не попадает в
// commit, как при kill). После перезапуска группы брокер доставляет событие
// снова, inbox отсекает повтор — эффект ровно один.
func TestCrashAfterCommitIsRedeliveredAndDeduplicated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "crash")
	db := pgtest.Fresh(t, "bus_test", fstest.MapFS{"001.sql": {Data: []byte(outbox.InboxSchema +
		`CREATE TABLE effects (event_id uuid PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());`)}})
	g1, g2 := uuid.NewString(), uuid.NewString()
	k.send(t, ctx, g1, g2)
	var mu sync.Mutex
	deliveries := map[string]int{}
	crashed := false
	handler := func(hctx context.Context, env events.Envelope, stop func()) error {
		mu.Lock()
		deliveries[env.ID]++
		mu.Unlock()
		err := pgx.BeginFunc(hctx, db, func(tx pgx.Tx) error {
			dup, err := outbox.Seen(hctx, tx, env)
			if err != nil || dup {
				return err
			}
			_, err = tx.Exec(hctx, `INSERT INTO effects(event_id) VALUES ($1)`, env.ID)
			return err
		})
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if env.ID == g1 && !crashed { // коммит прошёл — и процесс убит до ответа брокеру
			crashed = true
			stop()
			<-hctx.Done()
			return hctx.Err()
		}
		if deliveries[g2] > 0 {
			stop()
		}
		return nil
	}
	k.run(ctx, 15*time.Second, handler) // первый запуск: падение на g1
	if deliveries[g1] != 1 || deliveries[g2] != 0 {
		t.Fatalf("first run: %v", deliveries)
	}
	k.run(ctx, 15*time.Second, handler) // перезапуск
	if deliveries[g1] != 2 {
		t.Fatalf("event must be redelivered after crash, deliveries=%v", deliveries)
	}
	var effects, inbox int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM effects), (SELECT count(*) FROM inbox)`).Scan(&effects, &inbox); err != nil {
		t.Fatal(err)
	}
	if effects != 2 || inbox != 2 {
		t.Fatalf("inbox must keep each effect exactly once: effects=%d inbox=%d deliveries=%v", effects, inbox, deliveries)
	}
}

func TestIsPermanent(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("%w: bad", Permanent), true},
		{&pgconn.PgError{Code: "22P02"}, true},                                                     // invalid_text_representation: «abc» вместо UUID
		{&pgconn.PgError{Code: "23503"}, true},                                                     // нарушение внешнего ключа
		{&pgconn.PgError{Code: "40001"}, false},                                                    // serialization_failure — повторить
		{&pgconn.PgError{Code: "42501", Message: "permission denied for table order_feed"}, false}, // нет прав — ошибка настройки, событие ждёт
		{&pgconn.PgError{Code: "42501", Routine: "ExecWithCheckOptions", Message: "новая строка нарушает политику защиты на уровне строк"}, true}, // текст переведён — решает функция сервера
		{errors.New("connection refused"), false},
	} {
		if got := IsPermanent(c.err); got != c.want {
			t.Errorf("IsPermanent(%v) = %v", c.err, got)
		}
	}
}

// Ошибка данных из базы (корректный JSON, но значение не лезет в столбец) —
// в DLQ, следующее событие обрабатывается, партиция не стоит.
func TestDataErrorGoesToDLQ(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "dataerr")
	g1, g2 := uuid.NewString(), uuid.NewString()
	k.send(t, ctx, g1, g2)
	var mu sync.Mutex
	var handled []string
	calls := 0
	k.run(ctx, 15*time.Second, func(_ context.Context, env events.Envelope, stop func()) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if env.ID == g1 {
			return &pgconn.PgError{Code: "22P02", Message: `invalid input syntax for type uuid: "abc"`}
		}
		handled = append(handled, env.ID)
		stop()
		return nil
	})
	if len(handled) != 1 || handled[0] != g2 || calls != 2 {
		t.Fatalf("data error must not be retried or block the partition: handled=%v calls=%d", handled, calls)
	}
	if n := k.dlqCount(t, ctx); n != 1 {
		t.Fatalf("data error must land in DLQ once, got %d", n)
	}
}

// ---- РХД ПР-5, вариант А: границы подтверждений и устойчивость к дубликатам ----

// А-1. Одно и то же сохранённое событие доставлено шесть раз подряд (первая
// доставка и пять повторов). У каждой доставки своё смещение в брокере, а
// обработчик выполняет полезное действие один раз: остальные пять раз inbox
// отвечает «дубликат». В базе — одна отметка и один эффект.
func TestRepeatedDeliveryOffsetsAndDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "repeat")
	db := pgtest.Fresh(t, "bus_test", fstest.MapFS{"001.sql": {Data: []byte(outbox.InboxSchema +
		`CREATE TABLE effects (event_id uuid PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());`)}})
	id := uuid.NewString()
	k.send(t, ctx, id, id, id, id, id, id)
	var mu sync.Mutex
	var offsets []int64
	var statuses []string
	k.run(ctx, 20*time.Second, func(hctx context.Context, env events.Envelope, stop func()) error {
		status := "processed"
		err := pgx.BeginFunc(hctx, db, func(tx pgx.Tx) error {
			dup, err := outbox.Seen(hctx, tx, env)
			if err != nil {
				return err
			}
			if dup {
				status = "duplicate"
				return nil
			}
			_, err = tx.Exec(hctx, `INSERT INTO effects(event_id) VALUES ($1)`, env.ID)
			return err
		})
		if err != nil {
			return err
		}
		d, _ := DeliveryFrom(hctx)
		mu.Lock()
		offsets, statuses = append(offsets, d.Offset), append(statuses, status)
		n := len(offsets)
		mu.Unlock()
		if n == 6 {
			stop()
		}
		return nil
	})
	t.Logf("смещения доставок: %v", offsets)
	t.Logf("статусы обработчика: %v", statuses)
	if fmt.Sprint(offsets) != "[0 1 2 3 4 5]" {
		t.Fatalf("each delivery must have its own offset: %v", offsets)
	}
	if strings.Join(statuses, ",") != "processed,duplicate,duplicate,duplicate,duplicate,duplicate" {
		t.Fatalf("statuses: %v", statuses)
	}
	var effects, inbox int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM effects), (SELECT count(*) FROM inbox)`).Scan(&effects, &inbox); err != nil {
		t.Fatal(err)
	}
	t.Logf("в базе: inbox=%d, эффектов=%d", inbox, effects)
	if effects != 1 || inbox != 1 {
		t.Fatalf("want 1/1, got inbox=%d effects=%d", inbox, effects)
	}
	if n := k.dlqCount(t, ctx); n != 0 {
		t.Fatalf("duplicates must not go to DLQ, got %d", n)
	}
}

// А-2. Сбой после фиксации в базе: транзакция обработчика (inbox + эффект)
// зафиксирована, процесс остановлен до подтверждения смещения. После
// перезапуска группа читает ТО ЖЕ смещение, а не следующую запись; inbox
// отвечает «дубликат», и данные остаются согласованными: 1 отметка, 1 эффект
// на событие.
func TestCrashAfterCommitRereadsSameOffset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "sameoffset")
	db := pgtest.Fresh(t, "bus_test", fstest.MapFS{"001.sql": {Data: []byte(outbox.InboxSchema +
		`CREATE TABLE effects (event_id uuid PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());`)}})
	g1, g2 := uuid.NewString(), uuid.NewString()
	k.send(t, ctx, g1, g2)
	type seen struct {
		run    int
		offset int64
		status string
	}
	var mu sync.Mutex
	var log []seen
	runNo, crashed := 1, false
	handler := func(hctx context.Context, env events.Envelope, stop func()) error {
		status := "processed"
		err := pgx.BeginFunc(hctx, db, func(tx pgx.Tx) error {
			dup, err := outbox.Seen(hctx, tx, env)
			if err != nil {
				return err
			}
			if dup {
				status = "duplicate"
				return nil
			}
			_, err = tx.Exec(hctx, `INSERT INTO effects(event_id) VALUES ($1)`, env.ID)
			return err
		})
		if err != nil {
			return err
		}
		d, _ := DeliveryFrom(hctx)
		mu.Lock()
		log = append(log, seen{runNo, d.Offset, status})
		mu.Unlock()
		if env.ID == g1 && !crashed { // -fail-after-db: коммит в базу есть, подтверждения брокеру нет
			crashed = true
			stop()
			<-hctx.Done()
			return hctx.Err()
		}
		if env.ID == g2 {
			stop()
		}
		return nil
	}
	k.run(ctx, 15*time.Second, handler)
	runNo = 2
	k.run(ctx, 15*time.Second, handler)
	for _, s := range log {
		t.Logf("запуск %d: смещение %d → %s", s.run, s.offset, s.status)
	}
	want := []seen{{1, 0, "processed"}, {2, 0, "duplicate"}, {2, 1, "processed"}}
	if fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("deliveries: %v, want %v", log, want)
	}
	var effects, inbox int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM effects), (SELECT count(*) FROM inbox)`).Scan(&effects, &inbox); err != nil {
		t.Fatal(err)
	}
	if effects != 2 || inbox != 2 {
		t.Fatalf("want one mark and one effect per event: inbox=%d effects=%d", inbox, effects)
	}
}

// А-3. Тайм-аут базы — не ошибка контракта сообщения. Обработчик возвращает
// context.DeadlineExceeded: потребитель делает только попытки записи в базу
// (calls = ["db", "db", …]), в DLQ сообщение не уходит и смещение не
// подтверждается — после «починки базы» следующий запуск получает то же
// смещение 0 и обрабатывает событие.
func TestDeadlineExceededIsNotAContractError(t *testing.T) {
	if IsPermanent(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded must be a transient error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := newKafka(t, ctx, "deadline")
	id := uuid.NewString()
	k.send(t, ctx, id)
	var mu sync.Mutex
	var calls []string
	k.run(ctx, 20*time.Second, func(hctx context.Context, env events.Envelope, stop func()) error {
		mu.Lock()
		calls = append(calls, "db")
		n := len(calls)
		mu.Unlock()
		if n == 4 { // четырёх попыток достаточно: «база» так и не ответила, останавливаем процесс
			go func() { time.Sleep(100 * time.Millisecond); stop() }()
		}
		return fmt.Errorf("insert into inbox: %w", context.DeadlineExceeded) // база не ответила вовремя
	})
	t.Logf("calls = %v", calls)
	if len(calls) < 2 {
		t.Fatalf("transient error must be retried, calls=%v", calls)
	}
	for _, c := range calls {
		if c != "db" {
			t.Fatalf("only database attempts are expected, calls=%v", calls)
		}
	}
	if n := k.dlqCount(t, ctx); n != 0 {
		t.Fatalf("a database timeout must not send the message to DLQ, got %d", n)
	}
	// Смещение не подтверждено: новый запуск той же группы читает его снова.
	var offset int64 = -1
	var got string
	k.run(ctx, 15*time.Second, func(hctx context.Context, env events.Envelope, stop func()) error {
		d, _ := DeliveryFrom(hctx)
		offset, got = d.Offset, env.ID
		stop()
		return nil
	})
	t.Logf("после восстановления базы: смещение %d, событие то же: %v", offset, got == id)
	if offset != 0 || got != id {
		t.Fatalf("the same record must be redelivered: offset=%d id=%s", offset, got)
	}
}
