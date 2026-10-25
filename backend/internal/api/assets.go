package api

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

// Asset 是首页行情卡片的一项。
type Asset struct {
	Name   string    `json:"name"`
	Code   string    `json:"code"`
	Price  string    `json:"price"`
	Change string    `json:"change"`
	Up     bool      `json:"up"`
	Points []float64 `json:"points"` // 当日走势，用来画卡片里的小走势图；没有实时数据时为空
}

// QuoteFetcher 是行情源，生产环境用 market.Yahoo，测试里可以换成假的。
type QuoteFetcher interface {
	Quote(ctx context.Context, symbol string) (market.Quote, error)
}

type tracked struct {
	symbol string
	sample Asset // 拉不到行情时的兜底数据，与前端设计稿保持一致
}

var trackedAssets = []tracked{
	{"^GSPC", Asset{Name: "标普 500", Code: "S&P 500", Price: "5,842.3", Change: "+0.62%", Up: true}},
	{"GC=F", Asset{Name: "现货黄金", Code: "XAU / USD", Price: "2,687.4", Change: "+1.84%", Up: true}},
	{"CL=F", Asset{Name: "WTI 原油", Code: "CL1!", Price: "68.92", Change: "-2.15%", Up: false}},
}

const assetsTTL = time.Minute

// assetService 缓存行情，避免每个页面请求都去打 Yahoo。
// 某个品种拉取失败时沿用它上一次的成功结果，从没成功过就用示例数据。
type assetService struct {
	quotes QuoteFetcher

	mu        sync.Mutex
	cached    []Asset
	fetchedAt time.Time
}

func (s *assetService) list(ctx context.Context) []Asset {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && time.Since(s.fetchedAt) < assetsTTL {
		return s.cached
	}

	next := make([]Asset, len(trackedAssets))
	for i, t := range trackedAssets {
		next[i] = t.sample
		if s.cached != nil {
			next[i] = s.cached[i]
		}
	}
	if s.quotes != nil {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var wg sync.WaitGroup
		for i, t := range trackedAssets {
			wg.Go(func() {
				q, err := s.quotes.Quote(ctx, t.symbol)
				if err != nil {
					log.Printf("assets: %v", err)
					return
				}
				next[i] = fromQuote(t.sample, q)
			})
		}
		wg.Wait()
	}

	s.cached, s.fetchedAt = next, time.Now()
	return next
}

func fromQuote(base Asset, q market.Quote) Asset {
	var pct float64
	if q.PrevClose != 0 {
		pct = (q.Price/q.PrevClose - 1) * 100
	}
	base.Price = formatPrice(q.Price)
	base.Change = fmt.Sprintf("%+.2f%%", pct)
	base.Up = pct >= 0
	base.Points = q.Points
	return base
}

// formatPrice 保留两位小数并加千分位，如 7623.68 → "7,623.68"。
func formatPrice(v float64) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if v < 0 {
		b.WriteByte('-')
	}
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + "." + frac
}
