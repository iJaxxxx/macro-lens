package api

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
	"github.com/iJaxxxx/macro-lens/backend/internal/quant"
)

type factorDef struct {
	symbol string
	name   string // 归因条形图里的名字
	tile   string // 宏观因子卡片上的名字
	kind   quant.FactorKind
	format func(float64) string
}

var factorDefs = []factorDef{
	{"^TNX", "美债利率", "10年期美债收益率", quant.FactorDiffBP, func(v float64) string { return fmt.Sprintf("%.2f%%", v) }},
	{"DX-Y.NYB", "美元", "美元指数 DXY", quant.FactorReturn, func(v float64) string { return fmt.Sprintf("%.2f", v) }},
	{"^VIX", "风险情绪", "VIX 恐慌指数", quant.FactorReturn, func(v float64) string { return fmt.Sprintf("%.2f", v) }},
	{"HG=F", "增长预期", "COMEX 铜", quant.FactorReturn, func(v float64) string { return fmt.Sprintf("%.3f", v) }},
	{"^GSPC", "风险偏好", "标普 500", quant.FactorReturn, formatPrice},
	{"BZ=F", "布伦特原油", "布伦特原油", quant.FactorReturn, func(v float64) string { return fmt.Sprintf("%.2f", v) }},
	{"^IRX", "短端利率", "3个月美债收益率", quant.FactorDiffBP, func(v float64) string { return fmt.Sprintf("%.2f%%", v) }},
}

// 宏观因子卡片展示的顺序
var factorTiles = []string{"^TNX", "^IRX", "DX-Y.NYB", "^VIX", "HG=F", "BZ=F"}

func findFactor(symbol string) factorDef {
	for _, f := range factorDefs {
		if f.symbol == symbol {
			return f
		}
	}
	panic("unknown factor " + symbol)
}

type FactorTile struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Day       string `json:"day"`  // 今日变化
	Week      string `json:"week"` // 近 5 个交易日变化
	DayDir    string `json:"dayDir"`
	WeekDir   string `json:"weekDir"`
	Available bool   `json:"available"`
}

// describeMove 把因子的变化写成人能读的形式：收益率类用基点，价格类用百分比。
func describeMove(kind quant.FactorKind, move float64) string {
	if kind == quant.FactorDiffBP {
		return fmt.Sprintf("%+.0fbp", move)
	}
	return fmt.Sprintf("%+.2f%%", move*100)
}

func direction(move, deadband float64) string {
	switch {
	case move > deadband:
		return "up"
	case move < -deadband:
		return "down"
	}
	return "flat"
}

func arrow(dir string) string {
	return map[string]string{"up": "↑ ", "down": "↓ ", "flat": "→ "}[dir]
}

// factorMove 返回最后一根 K 线相对 n 根之前的变化。
func factorMove(kind quant.FactorKind, bars []market.Bar, n int) float64 {
	if len(bars) <= n {
		return math.NaN()
	}
	a, b := bars[len(bars)-1-n].Close, bars[len(bars)-1].Close
	if kind == quant.FactorDiffBP {
		return (b - a) * 100
	}
	return b/a - 1
}

func listFactors(ctx context.Context, m *marketCache) []FactorTile {
	out := make([]FactorTile, len(factorTiles))
	var wg sync.WaitGroup
	for i, sym := range factorTiles {
		f := findFactor(sym)
		out[i] = FactorTile{Symbol: sym, Name: f.tile, Value: "—", Day: "暂无数据", Week: "暂无数据", DayDir: "flat", WeekDir: "flat"}
		if m == nil {
			continue
		}
		wg.Go(func() {
			bars, err := m.liveHistory(ctx, sym)
			if err != nil || len(bars) < 6 {
				return
			}
			t := &out[i]
			// 基点变化 1bp 以内、涨跌 0.05% 以内视为持平
			deadband := 0.0005
			if f.kind == quant.FactorDiffBP {
				deadband = 1
			}
			day, week := factorMove(f.kind, bars, 1), factorMove(f.kind, bars, 5)
			t.Value = f.format(bars[len(bars)-1].Close)
			t.DayDir, t.WeekDir = direction(day, deadband), direction(week, deadband)
			t.Day = arrow(t.DayDir) + describeMove(f.kind, day)
			t.Week = arrow(t.WeekDir) + describeMove(f.kind, week)
			t.Available = true
		})
	}
	wg.Wait()
	return out
}
