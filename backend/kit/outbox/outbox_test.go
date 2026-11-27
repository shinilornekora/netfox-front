package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/kit/pgtest"
)

type pub struct {
	mu   sync.Mutex
	got  []string
	fail bool
}

func (p *pub) Publish(_ context.Context, msgs []outbox.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return errors.New("broker down")
	}
	for _, m := range msgs {
		p.got = append(p.got, m.Key)
	}
	return nil
}

func add(t *testing.T, db pgkit.Q, n int) {
	ctx := context.Background()
	pool := db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	for i := 0; i < n; i++ {
		tx, _ := pool.Begin(ctx)
		if err := outbox.Add(ctx, tx, "t", "test", "x", string(rune('a'+i%26)), int64(i), "", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(ctx)
	}
}

// Ретранслятор «упал» после того, как взял записи в аренду: пока аренда не
// истекла, другой экземпляр их не трогает; после — забирает и доставляет.
func TestLeaseExpiryRedelivers(t *testing.T) {
	db := pgtest.Fresh(t, "outbox_test", fstest.MapFS{"001.sql": {Data: []byte(outbox.Schema)}})
	add(t, db, 3)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	crashed := &pub{fail: true}
	a := &outbox.Relay{Pool: db, Pub: crashed, Log: log, Owner: "A", Lease: 300 * time.Millisecond}
	if n, err := a.Once(ctx); n != 0 || err == nil {
		t.Fatalf("A fails to publish: n=%d err=%v", n, err)
	}
	healthy := &pub{}
	b := &outbox.Relay{Pool: db, Pub: healthy, Log: log, Owner: "B", Lease: time.Second}
	if n, _ := b.Once(ctx); n != 0 {
		t.Fatal("B must not take records leased by A")
	}
	time.Sleep(350 * time.Millisecond)
	if n, err := b.Once(ctx); n != 3 || err != nil {
		t.Fatalf("after lease expiry B delivers: n=%d err=%v", n, err)
	}
	var pending, attempts int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE sent_at IS NULL), max(attempts) FROM outbox`).Scan(&pending, &attempts); err != nil {
		t.Fatal(err)
	}
	if pending != 0 || attempts != 2 {
		t.Fatalf("pending=%d attempts=%d", pending, attempts)
	}
	if healthy.got[0] != "a" || healthy.got[2] != "c" {
		t.Fatalf("order by seq: %v", healthy.got)
	}
}

// Два экземпляра параллельно: каждое событие доставлено ровно одним.
func TestConcurrentRelaysDoNotDuplicate(t *testing.T) {
	db := pgtest.Fresh(t, "outbox_test", fstest.MapFS{"001.sql": {Data: []byte(outbox.Schema)}})
	add(t, db, 200)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := &pub{}
	var wg sync.WaitGroup
	for _, owner := range []string{"A", "B", "C"} {
		wg.Add(1)
		go func(o string) {
			defer wg.Done()
			r := &outbox.Relay{Pool: db, Pub: p, Log: log, Owner: o, Batch: 17}
			for {
				n, err := r.Once(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if n == 0 {
					return
				}
			}
		}(owner)
	}
	wg.Wait()
	if len(p.got) != 200 {
		t.Fatalf("published %d of 200", len(p.got))
	}
}
