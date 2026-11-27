// Package resil — circuit breaker по доле ошибок в скользящем окне.
//
// Счётчик «N отказов подряд» не замечает частичную деградацию: если падает
// каждый третий вызов, подряд их никогда не бывает пять. Здесь решение
// принимается по доле ошибок среди последних Window вызовов.
package resil

import (
	"errors"
	"sync"
	"time"
)

var ErrOpen = errors.New("circuit open")

type Breaker struct {
	Window    int           // сколько последних вызовов учитывать
	MinCalls  int           // меньше — статистики недостаточно
	Threshold float64       // доля ошибок для размыкания (0..1)
	Cooldown  time.Duration // сколько держать разомкнутым
	Now       func() time.Time

	mu       sync.Mutex
	ring     []bool // true — ошибка
	pos, n   int
	openedAt time.Time
	state    int // 0 closed, 1 open, 2 half-open
	probing  bool
}

func New(window, minCalls int, threshold float64, cooldown time.Duration) *Breaker {
	return &Breaker{Window: window, MinCalls: minCalls, Threshold: threshold, Cooldown: cooldown, ring: make([]bool, window)}
}

func (b *Breaker) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Allow — можно ли сделать вызов.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case 1:
		if b.now().Sub(b.openedAt) < b.Cooldown {
			return ErrOpen
		}
		b.state = 2
		fallthrough
	case 2:
		if b.probing {
			return ErrOpen
		}
		b.probing = true
	}
	return nil
}

// Record — результат вызова.
func (b *Breaker) Record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == 2 {
		b.probing = false
		if failed {
			b.state, b.openedAt = 1, b.now()
			return
		}
		b.state, b.pos, b.n = 0, 0, 0
		return
	}
	b.ring[b.pos] = failed
	b.pos = (b.pos + 1) % b.Window
	if b.n < b.Window {
		b.n++
	}
	if b.n < b.MinCalls {
		return
	}
	bad := 0
	for i := 0; i < b.n; i++ {
		if b.ring[i] {
			bad++
		}
	}
	if float64(bad)/float64(b.n) >= b.Threshold {
		b.state, b.openedAt, b.pos, b.n = 1, b.now(), 0, 0
	}
}

func (b *Breaker) Open() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state == 1 && b.now().Sub(b.openedAt) < b.Cooldown
}
