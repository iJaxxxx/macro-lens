package api

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

type fakeQuotes struct {
	calls atomic.Int32
	fail  map[string]bool
}

func (f *fakeQuotes) Quote(_ context.Context, symbol string) (market.Quote, error) {
	f.calls.Add(1)
	if f.fail[symbol] {
		return market.Quote{}, errors.New("boom")
	}
	return market.Quote{Price: 7623.68, PrevClose: 7663.43, Points: []float64{7660, 7623.68}}, nil
}

func TestAssetServiceLiveAndFallback(t *testing.T) {
	quotes := &fakeQuotes{fail: map[string]bool{"CL=F": true}}
	s := &assetService{quotes: quotes}
	got := s.list(context.Background())

	if got[0].Price != "7,623.68" || got[0].Change != "-0.52%" || got[0].Up || len(got[0].Points) != 2 {
		t.Errorf("live asset = %+v", got[0])
	}
	// 原油拉取失败，用示例数据兜底
	if !reflect.DeepEqual(got[2], trackedAssets[2].sample) {
		t.Errorf("fallback asset = %+v", got[2])
	}

	// TTL 内不再请求行情源
	s.list(context.Background())
	if n := quotes.calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3 (cached)", n)
	}
}

func TestAssetServiceKeepsLastGood(t *testing.T) {
	quotes := &fakeQuotes{fail: map[string]bool{}}
	s := &assetService{quotes: quotes}
	s.list(context.Background())

	// 缓存过期后行情源全部失败，应沿用上一次的实时数据，而不是退回示例数据
	s.fetchedAt = s.fetchedAt.Add(-2 * assetsTTL)
	quotes.fail = map[string]bool{"^GSPC": true, "GC=F": true, "CL=F": true}
	if got := s.list(context.Background()); got[0].Price != "7,623.68" {
		t.Errorf("price = %q, want last good value", got[0].Price)
	}
}

func TestFormatPrice(t *testing.T) {
	for v, want := range map[float64]string{7623.68: "7,623.68", 68.9: "68.90", 1234567.891: "1,234,567.89", -1500: "-1,500.00"} {
		if got := formatPrice(v); got != want {
			t.Errorf("formatPrice(%v) = %q, want %q", v, got, want)
		}
	}
}
