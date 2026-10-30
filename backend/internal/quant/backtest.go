package quant

import (
	"math"
)

// Strategy 根据截至当天（含）的收盘价决定第二天是否持仓。只做多或空仓，不加杠杆。
type Strategy struct {
	Key         string
	Name        string
	Description string
	Warmup      int // 需要多少根 K 线才能给出信号
	// Signal 拿到 closes[:t+1] 和前一天的仓位，返回新仓位（0 或 1）
	Signal func(closes []float64, prev float64) float64
}

var Strategies = []Strategy{
	{"buyhold", "买入持有", "一直满仓持有，作为基准", 1, func([]float64, float64) float64 { return 1 }},
	{"sma_cross", "均线交叉 20/60", "20 日均线在 60 日均线上方时持有，否则空仓", 60, func(c []float64, _ float64) float64 {
		return bool01(SMA(c, 20) > SMA(c, 60))
	}},
	{"trend200", "200 日趋势过滤", "收盘价站上 200 日均线时持有，跌破则空仓", 200, func(c []float64, _ float64) float64 {
		return bool01(c[len(c)-1] > SMA(c, 200))
	}},
	{"momentum", "时序动量 60 日", "过去 60 个交易日上涨则持有，下跌则空仓", 61, func(c []float64, _ float64) float64 {
		return bool01(Change(c, 60) > 0)
	}},
	{"macd", "MACD 柱线", "MACD 柱线为正时持有，为负时空仓", 35, func(c []float64, _ float64) float64 {
		_, _, h := MACD(c)
		return bool01(h > 0)
	}},
	{"rsi_reversion", "RSI 均值回归", "RSI(14) 跌破 30 时买入，回升到 55 以上卖出", 15, func(c []float64, prev float64) float64 {
		r := RSI(c, 14)
		switch {
		case prev == 0 && r < 30:
			return 1
		case prev == 1 && r > 55:
			return 0
		}
		return prev
	}},
}

func bool01(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// 每次换仓收取的单边成本（佣金 + 滑点），5 个基点
const tradeCost = 0.0005

type BacktestResult struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TotalReturn float64   `json:"totalReturn"`
	CAGR        float64   `json:"cagr"`
	Vol         float64   `json:"vol"`
	Sharpe      float64   `json:"sharpe"`
	MaxDrawdown float64   `json:"maxDrawdown"`
	Trades      int       `json:"trades"`
	WinRate     float64   `json:"winRate"`  // 已平仓交易里赚钱的比例
	Exposure    float64   `json:"exposure"` // 持仓天数占比
	Position    float64   `json:"position"` // 最新信号：1 持有，0 空仓
	Equity      []float64 `json:"equity"`   // 抽样后的净值曲线，起点为 1
}

// Backtest 从 start 开始逐日回测。所有策略用同一个 start，结果才可比。
func Backtest(s Strategy, closes []float64, start int) BacktestResult {
	res := BacktestResult{Key: s.Key, Name: s.Name, Description: s.Description}
	if start < s.Warmup-1 {
		start = s.Warmup - 1
	}
	if start >= len(closes)-1 {
		return res
	}

	equity := []float64{1}
	var daily []float64
	pos, held := 0.0, 0
	entry, wins, closed := 0.0, 0, 0
	for t := start; t < len(closes)-1; t++ {
		next := s.Signal(closes[:t+1], pos)
		cost := math.Abs(next-pos) * tradeCost
		if next > pos {
			res.Trades++
			entry = closes[t]
		} else if next < pos {
			closed++
			if closes[t] > entry {
				wins++
			}
		}
		pos = next
		r := pos*(closes[t+1]/closes[t]-1) - cost
		daily = append(daily, r)
		equity = append(equity, equity[len(equity)-1]*(1+r))
		if pos > 0 {
			held++
		}
	}
	res.Position = s.Signal(closes, pos)

	n := float64(len(daily))
	final := equity[len(equity)-1]
	res.TotalReturn = final - 1
	res.CAGR = math.Pow(final, 252/n) - 1
	res.Vol = StdDev(daily) * math.Sqrt(252)
	var mean float64
	for _, r := range daily {
		mean += r
	}
	mean /= n
	if sd := StdDev(daily); sd > 0 {
		res.Sharpe = mean / sd * math.Sqrt(252)
	}
	res.MaxDrawdown = MaxDrawdown(equity)
	res.Exposure = float64(held) / n
	if closed > 0 {
		res.WinRate = float64(wins) / float64(closed)
	}
	res.Equity = downsample(equity, 120)
	res.Vol, res.Sharpe, res.CAGR = Finite(res.Vol), Finite(res.Sharpe), Finite(res.CAGR)
	return res
}

func downsample(xs []float64, n int) []float64 {
	if len(xs) <= n {
		return xs
	}
	out := make([]float64, n)
	for i := range n {
		out[i] = xs[i*(len(xs)-1)/(n-1)]
	}
	return out
}

// TechnicalScore 把技术面综合成 -1（强烈看空）到 1（强烈看多）的分数。
// 规则引擎版智能体和预测成绩单都用它，保证两边口径一致。
func TechnicalScore(c []float64) float64 {
	if len(c) < 60 {
		return 0
	}
	p := c[len(c)-1]
	var score, weight float64
	add := func(w, v float64) { score += w * v; weight += w }

	add(1, sign(p-SMA(c, 20)))
	add(1, sign(SMA(c, 20)-SMA(c, 60)))
	if len(c) >= 200 {
		add(1, sign(p-SMA(c, 200)))
	}
	_, _, h := MACD(c)
	add(1, sign(h))
	add(1, sign(Change(c, 20)))
	// RSI 极端值按均值回归处理：超买减分、超卖加分
	switch r := RSI(c, 14); {
	case r > 70:
		add(1, -1)
	case r < 30:
		add(1, 1)
	}
	return score / weight
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// HitRate 统计最近 days 天里，用 TechnicalScore 预测次日涨跌方向的命中率。
// 分数为 0 的日子不算预测。
func HitRate(c []float64, days int) (hits, total int) {
	for t := max(60, len(c)-1-days); t < len(c)-1; t++ {
		s := TechnicalScore(c[:t+1])
		if s == 0 {
			continue
		}
		total++
		if sign(s) == sign(c[t+1]-c[t]) {
			hits++
		}
	}
	return hits, total
}
