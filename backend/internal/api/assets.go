package api

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

// Asset 是首页行情卡片的一项。
type Asset struct {
	Symbol    string     `json:"symbol"`
	Group     string     `json:"group"` // 美股 | 黄金 | 原油
	Name      string     `json:"name"`
	Code      string     `json:"code"`
	Price     string     `json:"price"`
	Change    string     `json:"change"`
	Up        bool       `json:"up"`
	Points    []float64  `json:"points"` // 当日走势，用来画卡片里的小走势图；没有实时数据时为空
	Live      bool       `json:"live"`   // false 表示拉不到行情，展示的是示例数据
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type tracked struct {
	symbol string
	sample Asset // 拉不到行情时的兜底数据
	// 归因用的因子：股票看利率、美元、风险情绪、铜（增长预期）；黄金看利率、美元、避险；原油看美元、增长和风险偏好
	factors []string
}

var equityFactors = []string{"^TNX", "DX-Y.NYB", "^VIX", "HG=F"}

var trackedAssets = []tracked{
	{"^GSPC", Asset{Group: "美股", Name: "标普 500", Code: "S&P 500", Price: "5,842.30", Change: "+0.62%", Up: true}, equityFactors},
	{"^IXIC", Asset{Group: "美股", Name: "纳斯达克", Code: "NASDAQ", Price: "18,567.19", Change: "+0.85%", Up: true}, equityFactors},
	{"^DJI", Asset{Group: "美股", Name: "道琼斯", Code: "DJIA", Price: "42,863.86", Change: "+0.31%", Up: true}, equityFactors},
	{"VOO", Asset{Group: "美股", Name: "VOO", Code: "Vanguard S&P 500 ETF", Price: "537.12", Change: "+0.61%", Up: true}, equityFactors},
	{"GC=F", Asset{Group: "黄金", Name: "黄金", Code: "COMEX 黄金期货", Price: "2,687.40", Change: "+1.84%", Up: true}, []string{"^TNX", "DX-Y.NYB", "^VIX"}},
	{"CL=F", Asset{Group: "原油", Name: "WTI 原油", Code: "NYMEX 原油期货", Price: "68.92", Change: "-2.15%", Up: false}, []string{"DX-Y.NYB", "HG=F", "^GSPC", "^VIX"}},
}

func init() {
	for i := range trackedAssets {
		trackedAssets[i].sample.Symbol = trackedAssets[i].symbol
	}
}

func findTracked(symbol string) (tracked, bool) {
	for _, t := range trackedAssets {
		if t.symbol == symbol {
			return t, true
		}
	}
	return tracked{}, false
}

func listAssets(ctx context.Context, m *marketCache) []Asset {
	out := make([]Asset, len(trackedAssets))
	var wg sync.WaitGroup
	for i, t := range trackedAssets {
		out[i] = t.sample
		if m == nil {
			continue
		}
		wg.Go(func() {
			if q, err := m.quotes.get(ctx, t.symbol); err == nil {
				out[i] = fromQuote(t.sample, q)
			}
		})
	}
	wg.Wait()
	return out
}

func changePct(q market.Quote) float64 {
	if q.PrevClose == 0 {
		return 0
	}
	return (q.Price/q.PrevClose - 1) * 100
}

func fromQuote(base Asset, q market.Quote) Asset {
	pct := changePct(q)
	base.Price = formatPrice(q.Price)
	base.Change = fmt.Sprintf("%+.2f%%", pct)
	base.Up = pct >= 0
	base.Points = q.Points
	base.Live = true
	if !q.Time.IsZero() {
		t := q.Time
		base.UpdatedAt = &t
	}
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
