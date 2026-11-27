// Command loadgen — генератор нагрузки для РХД ПР-1 в домене Плёнки.
//
// Замер идёт сквозь весь путь, как его проходит браузер: gateway (CSRF,
// сессия из кэша, лимиты, подпись личности) → сервис → PostgreSQL (RLS,
// транзакция). Конкурентность ограничена: ровно -workers горутин берут
// задания из канала; задание — номер, по нему детерминированно выбираются
// операция и пользователь, поэтому повторные опыты имеют то же соотношение.
//
//	go run ./loadgen -mix watch    -workers 8 -requests 2000 -runs 3
//	go run ./loadgen -mix checkout -workers 8 -requests 100  -runs 3
//
// watch:    80% PUT /api/playback/progress (пинг плеера, запись)
//
//	20% GET /api/playback/continue (лента «продолжить», чтение)
//
// checkout: POST /api/billing/checkout — покупка фильма (сага с банком).
//
// Пользователи регистрируются напрямую в identity (-identity), а не через
// gateway: лимит 10 входов/регистраций в минуту с одного IP — защита от
// подбора, а не часть измеряемого пути. Всё остальное — через gateway.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"slices"
	"sync"
	"time"
)

var (
	base     = flag.String("base", "http://localhost:8080", "gateway")
	identity = flag.String("identity", "", "identity напрямую (для регистрации); пусто — через gateway")
	mix      = flag.String("mix", "watch", "watch | checkout")
	workers  = flag.Int("workers", 8, "рабочих горутин")
	requests = flag.Int("requests", 2000, "заданий в одном опыте")
	runs     = flag.Int("runs", 3, "повторов")
	users    = flag.Int("users", 200, "зрителей для watch")
	film     = flag.String("film", "", "id фильма для watch (по подписке)")
	paid     = flag.String("paid", "", "id платного фильма для checkout")
	timeout  = flag.Duration("timeout", 60*time.Second, "предел одного опыта")
)

var client = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
	MaxIdleConns: 256, MaxIdleConnsPerHost: 256, IdleConnTimeout: time.Minute, Proxy: nil}}

type viewer struct{ cookie string }

func call(ctx context.Context, url, method, cookie string, body any, hdr map[string]string) (int, []byte, http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("X-Plenka-Client", "loadgen")
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", "plenka_sid="+cookie)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body) // время — до последнего байта ответа
	return resp.StatusCode, b, resp.Header, err
}

func key() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func register(ctx context.Context, n int, tag string) []viewer {
	reg := *identity
	if reg == "" {
		reg = *base
	}
	out := make([]viewer, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var mu sync.Mutex
	var firstErr error
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			email := fmt.Sprintf("load-%s-%04d-%s@plenka.local", tag, i, key()[:6])
			st, b, h, err := call(ctx, reg+"/api/auth/register", "POST", "", map[string]any{
				"email": email, "password": "load-pass-2026", "display_name": "Нагрузка", "birth_date": "1990-01-01"}, nil)
			if err == nil && st != 201 && st != 200 {
				err = fmt.Errorf("register %d: %s", st, b)
			}
			if err != nil {
				mu.Lock()
				firstErr = err
				mu.Unlock()
				return
			}
			for _, c := range (&http.Response{Header: h}).Cookies() {
				if c.Name == "plenka_sid" {
					out[i].cookie = c.Value
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		fail("регистрация: %v", firstErr)
	}
	return out
}

func must(ctx context.Context, v viewer, method, path string, body any, hdr map[string]string, want ...int) []byte {
	st, b, _, err := call(ctx, *base+path, method, v.cookie, body, hdr)
	if err != nil || !slices.Contains(want, st) {
		fail("%s %s: %d %v %s", method, path, st, err, b)
	}
	return b
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+f+"\n", a...)
	os.Exit(1)
}

// ---------- статистика ----------

type sample struct {
	op  string
	d   time.Duration
	err bool
}

// nearest-rank: наименьшее значение, не меньше которого ≥ p% наблюдений.
func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	k := int(float64(len(sorted))*p/100+0.999999) - 1
	k = max(0, min(k, len(sorted)-1))
	return sorted[k]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d.Microseconds())/1000) }

func report(run int, wall time.Duration, ss []sample, notStarted int) {
	ops := map[string][]time.Duration{}
	errs := map[string]int{}
	var order []string
	okAll := 0
	for _, s := range ss {
		if _, seen := ops[s.op]; !seen {
			order = append(order, s.op)
			ops[s.op] = nil
		}
		if s.err {
			errs[s.op]++
			continue
		}
		ops[s.op] = append(ops[s.op], s.d)
		okAll++
	}
	slices.Sort(order)
	fmt.Printf("run=%d workers=%d wall=%s ok=%d throughput=%.0f/s not_started=%d\n",
		run, *workers, wall.Round(time.Millisecond), okAll, float64(okAll)/wall.Seconds(), notStarted)
	for _, op := range order {
		d := ops[op]
		slices.Sort(d)
		var sum time.Duration
		for _, x := range d {
			sum += x
		}
		mean := time.Duration(0)
		if len(d) > 0 {
			mean = sum / time.Duration(len(d))
		}
		fmt.Printf("  %-9s n=%-5d err=%-3d mean=%s p50=%s p95=%s p99=%s max=%s ms\n", op, len(d), errs[op],
			ms(mean), ms(pct(d, 50)), ms(pct(d, 95)), ms(pct(d, 99)), ms(pct(d, 100)))
	}
}

// ---------- сценарии ----------

func watchJob(ctx context.Context, vs []viewer, i int, now int64) sample {
	v := vs[i%len(vs)]
	if i%5 == 4 { // 20% чтений
		t0 := time.Now()
		st, _, _, err := call(ctx, *base+"/api/playback/continue", "GET", v.cookie, nil, nil)
		return sample{"continue", time.Since(t0), err != nil || st != 200}
	}
	pos := 10 + (i/len(vs))*10 // позиция растёт, как у настоящего плеера
	t0 := time.Now()
	st, _, _, err := call(ctx, *base+"/api/playback/progress", "PUT", v.cookie, map[string]any{
		"title_id": *film, "position_sec": pos, "duration_sec": 4680, "client_ms": now + int64(i)}, nil)
	return sample{"progress", time.Since(t0), err != nil || st != 200}
}

func checkoutJob(ctx context.Context, vs []viewer, i int) sample {
	v := vs[i]
	t0 := time.Now()
	st, _, _, err := call(ctx, *base+"/api/billing/checkout", "POST", v.cookie, map[string]any{
		"kind": "purchase", "title_id": *paid, "card_number": "4242424242424242"},
		map[string]string{"Idempotency-Key": key()})
	return sample{"checkout", time.Since(t0), err != nil || st != 200 && st != 201}
}

func main() {
	flag.Parse()
	ctx := context.Background()
	fmt.Printf("# loadgen mix=%s workers=%d requests=%d runs=%d go=%s cpu=%d date=%s\n",
		*mix, *workers, *requests, *runs, runtime.Version(), runtime.NumCPU(), time.Now().Format(time.RFC3339))

	var vs []viewer
	switch *mix {
	case "watch":
		if *film == "" {
			fail("-film обязателен")
		}
		vs = register(ctx, *users, "w")
		plan := map[string]any{}
		_ = json.Unmarshal(must(ctx, vs[0], "GET", "/api/billing/plans", nil, nil, 200), &plan)
		var planID string
		if items, ok := plan["items"].([]any); ok && len(items) > 0 {
			planID, _ = items[0].(map[string]any)["id"].(string)
		}
		for _, v := range vs { // подписка и старт просмотра — подготовка, не замер
			must(ctx, v, "POST", "/api/billing/checkout", map[string]any{"kind": "subscription", "plan_id": planID,
				"card_number": "4242424242424242"}, map[string]string{"Idempotency-Key": key()}, 200, 201)
			must(ctx, v, "POST", "/api/playback/start", map[string]any{"title_id": *film}, nil, 200)
		}
		// Прогрев: по одному запросу каждого вида на пользователя — прогреваются
		// кэш сессий gateway, кэш решений о доступе и соединения пулов.
		for _, v := range vs {
			must(ctx, v, "GET", "/api/playback/continue", nil, nil, 200)
		}
	case "checkout":
		if *paid == "" {
			fail("-paid обязателен")
		}
		vs = register(ctx, *requests**runs, "c")
		for _, v := range vs { // прогрев кэша сессий; покупок ещё нет
			must(ctx, v, "GET", "/api/billing/offers/"+*paid, nil, nil, 200)
		}
	default:
		fail("неизвестный mix %q", *mix)
	}

	for run := 1; run <= *runs; run++ {
		rctx, cancel := context.WithTimeout(ctx, *timeout)
		jobs := make(chan int)
		res := make(chan sample, *requests)
		var wg sync.WaitGroup
		now := time.Now().UnixMilli()
		for range *workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					switch *mix {
					case "watch":
						res <- watchJob(rctx, vs, (run-1)**requests+i, now)
					case "checkout":
						res <- checkoutJob(rctx, vs, (run-1)**requests+i)
					}
				}
			}()
		}
		t0 := time.Now()
		sent := 0
	feed:
		for i := range *requests {
			select {
			case jobs <- i:
				sent++
			case <-rctx.Done():
				break feed
			}
		}
		close(jobs)
		wg.Wait()
		wall := time.Since(t0)
		cancel()
		close(res)
		var ss []sample
		for s := range res {
			ss = append(ss, s)
		}
		report(run, wall, ss, *requests-sent)
	}
}
