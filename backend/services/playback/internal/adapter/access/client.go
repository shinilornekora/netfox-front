// Package access — gRPC-клиент к billing (ПКП-4: проверка ресурса по gRPC).
//
// Поверх вызова три защиты:
//   - бюджет времени: зритель нажал «Смотреть» и ждёт, долго ждать нельзя;
//   - circuit breaker по доле ошибок: лежащий billing не копит очередь
//     зависших вызовов;
//   - кэш решений с «stale-if-error»: положительное решение живёт до конца
//     оплаченного периода (но не больше 5 минут), а если billing недоступен,
//     используем решение не старше 30 минут. Отрицательные решения не
//     кэшируем: зритель только что оплатил — и сразу должен смотреть.
package access

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	billingv1 "plenka/contracts/gen/billing/v1"
	"plenka/kit/resil"
)

type entry struct {
	via   string
	fresh time.Time // до этого момента можно не спрашивать billing
	stale time.Time // до этого момента можно использовать при отказе billing
}

type Client struct {
	rpc     billingv1.AccessClient
	conn    *grpc.ClientConn
	Budget  time.Duration
	Breaker *resil.Breaker
	Now     func() time.Time

	mu    sync.Mutex
	cache map[[2]uuid.UUID]entry
	// gen — поколение решений: любой Forget его увеличивает. Ответ billing,
	// запрошенный до сброса, в кэш уже не кладётся — иначе «да», полученное
	// за миг до возврата денег, прожило бы ещё 5 минут. Счётчик один на весь
	// кэш, а не на зрителя: памяти он не копит, а цена — редкий лишний вызов
	// billing у другого зрителя, чья проверка совпала с чужим возвратом.
	gen uint64
}

func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return New(billingv1.NewAccessClient(conn), conn), nil
}

func New(rpc billingv1.AccessClient, conn *grpc.ClientConn) *Client {
	return &Client{rpc: rpc, conn: conn, Budget: 500 * time.Millisecond,
		Breaker: resil.New(20, 8, 0.5, 15*time.Second), cache: map[[2]uuid.UUID]entry{}}
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) Check(ctx context.Context, user, title uuid.UUID) (bool, string, string, error) {
	k := [2]uuid.UUID{user, title}
	now := c.now()
	c.mu.Lock()
	e, cached := c.cache[k]
	gen := c.gen
	c.mu.Unlock()
	if cached && now.Before(e.fresh) {
		return true, e.via, "", nil
	}

	resp, err := c.call(ctx, user, title)
	if err != nil {
		if cached && now.Before(e.stale) {
			return true, e.via, "", nil // billing недоступен — доверяем недавнему «да»
		}
		return false, "", "", err
	}
	if !resp.GetAllowed() {
		c.mu.Lock()
		delete(c.cache, k)
		c.mu.Unlock()
		denial := "not_entitled"
		if resp.GetDenial() == billingv1.Denial_DENIAL_NOT_AVAILABLE {
			denial = "not_available"
		}
		return false, "", denial, nil
	}
	via := "subscription"
	if resp.GetGrant() == billingv1.Grant_GRANT_PURCHASE {
		via = "purchase"
	}
	fresh := now.Add(5 * time.Minute)
	if v := resp.GetValidUntilUnix(); v > 0 && time.Unix(v, 0).Before(fresh) {
		fresh = time.Unix(v, 0)
	}
	c.mu.Lock()
	if c.gen == gen { // пока billing отвечал, решения не сбрасывали
		c.cache[k] = entry{via: via, fresh: fresh, stale: now.Add(30 * time.Minute)}
		if len(c.cache) > 50_000 { // грубая защита памяти
			c.cache = map[[2]uuid.UUID]entry{k: c.cache[k]}
		}
	}
	c.mu.Unlock()
	return true, via, "", nil
}

func (c *Client) call(ctx context.Context, user, title uuid.UUID) (*billingv1.CheckResponse, error) {
	if err := c.Breaker.Allow(); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, c.Budget)
	defer cancel()
	resp, err := c.rpc.Check(cctx, &billingv1.CheckRequest{UserId: user.String(), TitleId: title.String()})
	c.Breaker.Record(err != nil)
	return resp, err
}

// Forget сбрасывает кэшированное решение по паре «зритель — тайтл».
func (c *Client) Forget(user, title uuid.UUID) {
	c.mu.Lock()
	delete(c.cache, [2]uuid.UUID{user, title})
	c.gen++
	c.mu.Unlock()
}

// ForgetUser сбрасывает все решения зрителя (подписка изменилась).
func (c *Client) ForgetUser(user uuid.UUID) {
	c.mu.Lock()
	for k := range c.cache {
		if k[0] == user {
			delete(c.cache, k)
		}
	}
	c.gen++
	c.mu.Unlock()
}
