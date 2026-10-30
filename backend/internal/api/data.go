package api

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

// MarketData 是行情源，生产环境用 market.Yahoo，测试里可以换成假的。
type MarketData interface {
	Quote(ctx context.Context, symbol string) (market.Quote, error)
	History(ctx context.Context, symbol string) ([]market.Bar, error)
}

const (
	quoteTTL   = 30 * time.Second
	historyTTL = 15 * time.Minute
	retryAfter = 20 * time.Second
)

// cache 按 key 缓存一次拉取的结果。同一个 key 同时只有一个请求在拉，其余等待结果；
// 拉取失败时沿用上一次成功的值，从没成功过才返回错误。
type cache[T any] struct {
	ttl   time.Duration
	fetch func(ctx context.Context, key string) (T, error)

	mu      sync.Mutex
	entries map[string]*entry[T]
}

type entry[T any] struct {
	mu        sync.Mutex
	val       T
	ok        bool
	err       error
	fetchedAt time.Time
}

func newCache[T any](ttl time.Duration, fetch func(context.Context, string) (T, error)) *cache[T] {
	return &cache[T]{ttl: ttl, fetch: fetch, entries: map[string]*entry[T]{}}
}

func (c *cache[T]) get(ctx context.Context, key string) (T, error) {
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		e = &entry[T]{}
		c.entries[key] = e
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	// 上次失败的话只隔一小会儿就重试
	ttl := c.ttl
	if e.err != nil {
		ttl = min(ttl, retryAfter)
	}
	if !e.fetchedAt.IsZero() && time.Since(e.fetchedAt) < ttl {
		if e.ok {
			return e.val, nil
		}
		var zero T
		return zero, e.err
	}
	v, err := c.fetch(ctx, key)
	// 失败也记下时间，避免行情源挂掉时每个请求都去重试
	e.fetchedAt, e.err = time.Now(), err
	if err != nil {
		log.Printf("fetch %s: %v", key, err)
		if e.ok {
			return e.val, nil
		}
		var zero T
		return zero, err
	}
	e.val, e.ok = v, true
	return v, nil
}

// marketCache 给报价和日线各加一层缓存。
type marketCache struct {
	quotes  *cache[market.Quote]
	history *cache[[]market.Bar]
}

func newMarketCache(src MarketData) *marketCache {
	if src == nil {
		return nil
	}
	return &marketCache{
		quotes: newCache(quoteTTL, func(ctx context.Context, s string) (market.Quote, error) {
			// 结果是多个请求共享的，不能因为某个用户关掉页面就取消
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			return src.Quote(ctx, s)
		}),
		history: newCache(historyTTL, func(ctx context.Context, s string) ([]market.Bar, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			return src.History(ctx, s)
		}),
	}
}

// liveHistory 返回日线，并用最新报价更新最后一根 K 线，这样归因和指标都是实时的。
func (m *marketCache) liveHistory(ctx context.Context, symbol string) ([]market.Bar, error) {
	bars, err := m.history.get(ctx, symbol)
	if err != nil {
		return nil, err
	}
	q, err := m.quotes.get(ctx, symbol)
	if err != nil || q.Time.IsZero() || len(bars) == 0 {
		return bars, nil
	}
	date := market.TradingDate(q.Time)
	last := bars[len(bars)-1]
	switch {
	case date == last.Date:
		out := append([]market.Bar(nil), bars...)
		out[len(out)-1].Close = q.Price
		return out, nil
	case date > last.Date:
		return append(append([]market.Bar(nil), bars...), market.Bar{Date: date, Close: q.Price}), nil
	}
	return bars, nil
}

// histories 并发拉取多个品种的日线，任何一个失败都返回错误。
func (m *marketCache) histories(ctx context.Context, symbols ...string) (map[string][]market.Bar, error) {
	out := make(map[string][]market.Bar, len(symbols))
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for _, s := range symbols {
		wg.Go(func() {
			bars, err := m.liveHistory(ctx, s)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			out[s] = bars
		})
	}
	wg.Wait()
	return out, firstErr
}
