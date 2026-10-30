package api

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

// fakeMarket 给每个代码生成一条确定的随机游走日线，报价就是最后一根 K 线。
type fakeMarket struct {
	quoteCalls atomic.Int32
	mu         sync.Mutex
	fail       map[string]bool
}

func (f *fakeMarket) failing(symbol string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fail[symbol]
}

func (f *fakeMarket) setFail(symbols ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = map[string]bool{}
	for _, s := range symbols {
		f.fail[s] = true
	}
}

func fakeBars(symbol string) []market.Bar {
	h := fnv.New64a()
	h.Write([]byte(symbol))
	rng := rand.New(rand.NewPCG(h.Sum64(), 7))
	day := time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC)
	price := 100.0
	var bars []market.Bar
	for len(bars) < 800 {
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
			price *= 1 + rng.NormFloat64()*0.01 + 0.0003
			bars = append(bars, market.Bar{Date: day.Format(time.DateOnly), Close: price})
		}
		day = day.AddDate(0, 0, 1)
	}
	return bars
}

func (f *fakeMarket) Quote(_ context.Context, symbol string) (market.Quote, error) {
	f.quoteCalls.Add(1)
	if f.failing(symbol) {
		return market.Quote{}, errors.New("boom")
	}
	bars := fakeBars(symbol)
	last, prev := bars[len(bars)-1], bars[len(bars)-2]
	d, _ := time.ParseInLocation(time.DateOnly, last.Date, market.NewYork)
	return market.Quote{Price: last.Close, PrevClose: prev.Close, Points: []float64{prev.Close, last.Close}, Time: d.Add(16 * time.Hour)}, nil
}

func (f *fakeMarket) History(_ context.Context, symbol string) ([]market.Bar, error) {
	if f.failing(symbol) {
		return nil, errors.New("boom")
	}
	return fakeBars(symbol), nil
}

func TestListAssetsLiveAndFallback(t *testing.T) {
	src := &fakeMarket{}
	src.setFail("CL=F")
	m := newMarketCache(src)
	got := listAssets(context.Background(), m)

	if len(got) != len(trackedAssets) {
		t.Fatalf("len = %d", len(got))
	}
	if !got[0].Live || got[0].Symbol != "^GSPC" || len(got[0].Points) != 2 || got[0].UpdatedAt == nil {
		t.Errorf("live asset = %+v", got[0])
	}
	voo, _ := findTracked("VOO")
	if voo.sample.Name != "VOO" {
		t.Error("VOO not tracked")
	}
	// 原油拉取失败，用示例数据兜底
	oil := got[len(got)-1]
	if oil.Live || !reflect.DeepEqual(oil, trackedAssets[len(trackedAssets)-1].sample) {
		t.Errorf("fallback asset = %+v", oil)
	}

	// TTL 内不再请求行情源
	before := src.quoteCalls.Load()
	listAssets(context.Background(), m)
	if n := src.quoteCalls.Load(); n != before {
		t.Errorf("calls = %d, want %d (cached)", n, before)
	}
}

func TestCacheKeepsLastGood(t *testing.T) {
	calls := 0
	c := newCache(time.Minute, func(context.Context, string) (int, error) {
		calls++
		if calls > 1 {
			return 0, errors.New("down")
		}
		return 42, nil
	})
	c.get(context.Background(), "k")
	c.entries["k"].fetchedAt = time.Now().Add(-2 * time.Minute)
	if v, err := c.get(context.Background(), "k"); err != nil || v != 42 {
		t.Errorf("v = %d, err = %v, want last good value", v, err)
	}
	// 失败之后 retryAfter 内不重复请求
	c.get(context.Background(), "k")
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestLiveHistoryAppendsQuote(t *testing.T) {
	src := &fakeMarket{}
	m := newMarketCache(src)
	bars := fakeBars("^GSPC")
	// 日线只到前一天，报价是今天的
	m.history.entries["^GSPC"] = &entry[[]market.Bar]{val: bars[:len(bars)-1], ok: true, fetchedAt: time.Now()}
	got, err := m.liveHistory(context.Background(), "^GSPC")
	if err != nil || len(got) != len(bars) || got[len(got)-1] != bars[len(bars)-1] {
		t.Errorf("live history tail = %+v, err = %v", got[len(got)-1], err)
	}
}

func TestAttribution(t *testing.T) {
	m := newMarketCache(&fakeMarket{})
	tr, _ := findTracked("GC=F")
	v, err := computeAttribution(context.Background(), m, tr)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rows) != len(tr.factors)+1 || v.Rows[len(v.Rows)-1].Name != "其他" || v.Window != attributionWindow {
		t.Errorf("rows = %+v", v.Rows)
	}
	var sum, share float64
	for _, r := range v.Rows {
		sum += r.Contribution
		share += r.Share
	}
	if math.Abs(sum-v.Total) > 1e-12 || math.Abs(share-1) > 1e-9 {
		t.Errorf("sum = %v total = %v share = %v", sum, v.Total, share)
	}
	if v.Title == "" || v.Summary == "" || v.Source != "rule" {
		t.Errorf("narrative = %q / %q", v.Title, v.Summary)
	}
}

func TestNarrationMatches(t *testing.T) {
	e := &narration{text: "跌了", total: -0.004}
	if !e.matches(-0.005) {
		t.Error("small change should keep narration")
	}
	if e.matches(0.006) || e.matches(-0.01) {
		t.Error("reversed or much larger move should invalidate narration")
	}
}

func TestFormatPrice(t *testing.T) {
	for v, want := range map[float64]string{7623.68: "7,623.68", 68.9: "68.90", 1234567.891: "1,234,567.89", -1500: "-1,500.00"} {
		if got := formatPrice(v); got != want {
			t.Errorf("formatPrice(%v) = %q, want %q", v, got, want)
		}
	}
}
