package resil

import (
	"testing"
	"time"
)

func TestPartialDegradationOpens(t *testing.T) {
	now := time.Unix(0, 0)
	b := New(10, 6, 0.5, time.Second)
	b.Now = func() time.Time { return now }
	// Каждый второй вызов падает: «N подряд» этого бы не заметил.
	for i := 0; i < 6; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("call %d rejected early", i)
		}
		b.Record(i%2 == 0)
	}
	if b.Allow() != ErrOpen {
		t.Fatal("50% failures must open the breaker")
	}
	now = now.Add(1100 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Fatal("half-open probe must pass")
	}
	if b.Allow() != ErrOpen {
		t.Fatal("only one probe at a time")
	}
	b.Record(false)
	if err := b.Allow(); err != nil || b.Open() {
		t.Fatal("successful probe closes")
	}
}

func TestFewCallsDoNotOpen(t *testing.T) {
	b := New(10, 6, 0.5, time.Second)
	for i := 0; i < 5; i++ {
		b.Record(true)
	}
	if b.Allow() != nil {
		t.Fatal("below MinCalls the breaker stays closed")
	}
}

func TestFailedProbeReopens(t *testing.T) {
	now := time.Unix(0, 0)
	b := New(4, 2, 0.5, time.Second)
	b.Now = func() time.Time { return now }
	b.Record(true)
	b.Record(true)
	now = now.Add(2 * time.Second)
	_ = b.Allow()
	b.Record(true)
	if b.Allow() != ErrOpen {
		t.Fatal("failed probe reopens for another cooldown")
	}
}
