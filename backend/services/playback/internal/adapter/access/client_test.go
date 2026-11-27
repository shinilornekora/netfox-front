package access

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	billingv1 "plenka/contracts/gen/billing/v1"
)

// slowBilling отвечает «да», но первый ответ задерживает, пока тест не
// разрешит: в этот момент приходит событие возврата.
type slowBilling struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (b *slowBilling) Check(ctx context.Context, _ *billingv1.CheckRequest, _ ...grpc.CallOption) (*billingv1.CheckResponse, error) {
	if b.calls.Add(1) == 1 {
		close(b.entered)
		<-b.release
	}
	return &billingv1.CheckResponse{Allowed: true, Grant: billingv1.Grant_GRANT_PURCHASE}, nil
}

// Гонка «ответ billing против сброса»: проверка началась до возврата денег,
// событие возврата сбросило кэш, пока billing отвечал. Старое «да» не должно
// лечь в кэш — следующая проверка снова идёт в billing.
func TestForgetDuringCheckDoesNotCacheStaleGrant(t *testing.T) {
	b := &slowBilling{entered: make(chan struct{}), release: make(chan struct{})}
	c := New(b, nil)
	user, title := uuid.New(), uuid.New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if ok, _, _, err := c.Check(context.Background(), user, title); !ok || err != nil {
			t.Errorf("first check: %v %v", ok, err)
		}
	}()
	<-b.entered
	c.Forget(user, title) // событие возврата пришло, пока billing отвечал
	close(b.release)
	<-done
	if _, _, _, err := c.Check(context.Background(), user, title); err != nil {
		t.Fatal(err)
	}
	if n := b.calls.Load(); n != 2 {
		t.Fatalf("grant fetched before Forget must not be cached: billing calls = %d, want 2", n)
	}
	// Без сброса решение кэшируется как обычно.
	if _, _, _, err := c.Check(context.Background(), user, title); err != nil {
		t.Fatal(err)
	}
	if n := b.calls.Load(); n != 2 {
		t.Fatalf("fresh grant must be served from cache: billing calls = %d", n)
	}
}
