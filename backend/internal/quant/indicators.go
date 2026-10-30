// Package quant 是纯计算：技术指标、多因子回归、策略回测。不做任何网络请求，方便测试。
package quant

import (
	"math"
	"slices"
)

// Returns 返回简单日收益率，长度比输入少 1。
func Returns(closes []float64) []float64 {
	if len(closes) < 2 {
		return nil
	}
	out := make([]float64, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		out[i-1] = closes[i]/closes[i-1] - 1
	}
	return out
}

// SMA 返回最后 n 个值的均值，数据不足时返回 NaN。
func SMA(xs []float64, n int) float64 {
	if n <= 0 || len(xs) < n {
		return math.NaN()
	}
	var s float64
	for _, x := range xs[len(xs)-n:] {
		s += x
	}
	return s / float64(n)
}

// EMASeries 返回与输入等长的指数移动平均序列，首个值用第一个点初始化。
func EMASeries(xs []float64, n int) []float64 {
	out := make([]float64, len(xs))
	k := 2 / float64(n+1)
	for i, x := range xs {
		if i == 0 {
			out[i] = x
			continue
		}
		out[i] = x*k + out[i-1]*(1-k)
	}
	return out
}

// RSI 用 Wilder 平滑计算最后一个点的相对强弱指数。
func RSI(closes []float64, n int) float64 {
	if len(closes) <= n {
		return math.NaN()
	}
	var gain, loss float64
	for i := 1; i <= n; i++ {
		d := closes[i] - closes[i-1]
		gain += math.Max(d, 0)
		loss += math.Max(-d, 0)
	}
	gain, loss = gain/float64(n), loss/float64(n)
	for i := n + 1; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		gain = (gain*float64(n-1) + math.Max(d, 0)) / float64(n)
		loss = (loss*float64(n-1) + math.Max(-d, 0)) / float64(n)
	}
	if loss == 0 {
		return 100
	}
	return 100 - 100/(1+gain/loss)
}

// MACD 返回最后一个点的 DIF、DEA 和柱值（12/26/9）。
func MACD(closes []float64) (dif, dea, hist float64) {
	if len(closes) < 35 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	fast, slow := EMASeries(closes, 12), EMASeries(closes, 26)
	difs := make([]float64, len(closes))
	for i := range closes {
		difs[i] = fast[i] - slow[i]
	}
	deas := EMASeries(difs, 9)
	n := len(closes) - 1
	return difs[n], deas[n], difs[n] - deas[n]
}

// StdDev 是样本标准差。
func StdDev(xs []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	var mean float64
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

// Bollinger 返回最后一个点在 n 日布林带里的位置：0 是下轨，1 是上轨。
func Bollinger(closes []float64, n int) float64 {
	if len(closes) < n {
		return math.NaN()
	}
	w := closes[len(closes)-n:]
	mid, sd := SMA(w, n), StdDev(w)
	if sd == 0 {
		return 0.5
	}
	return (closes[len(closes)-1] - (mid - 2*sd)) / (4 * sd)
}

// AnnualVol 是最近 n 个日收益的年化波动率。
func AnnualVol(returns []float64, n int) float64 {
	if len(returns) < n {
		n = len(returns)
	}
	return StdDev(returns[len(returns)-n:]) * math.Sqrt(252)
}

// MaxDrawdown 返回净值曲线的最大回撤（正数，如 0.18 表示 18%）。
func MaxDrawdown(equity []float64) float64 {
	var peak, dd float64
	for _, v := range equity {
		peak = math.Max(peak, v)
		if peak > 0 {
			dd = math.Max(dd, 1-v/peak)
		}
	}
	return dd
}

// Change 返回最后一个点相对 n 个点之前的涨跌幅。
func Change(closes []float64, n int) float64 {
	if len(closes) <= n {
		return math.NaN()
	}
	return closes[len(closes)-1]/closes[len(closes)-1-n] - 1
}

// Percentile 返回 x 在 xs 中的分位（0~1）。
func Percentile(xs []float64, x float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	i, _ := slices.BinarySearch(s, x)
	return float64(i) / float64(len(s))
}

// Snapshot 汇总一个品种最新的技术面数据，供智能体和前端使用。
type Snapshot struct {
	Price      float64 `json:"price"`
	Change1D   float64 `json:"change1d"`
	Change5D   float64 `json:"change5d"`
	Change20D  float64 `json:"change20d"`
	Change60D  float64 `json:"change60d"`
	SMA20      float64 `json:"sma20"`
	SMA50      float64 `json:"sma50"`
	SMA200     float64 `json:"sma200"`
	RSI14      float64 `json:"rsi14"`
	MACDHist   float64 `json:"macdHist"`
	MACDDif    float64 `json:"macdDif"`
	Bollinger  float64 `json:"bollinger"`
	Vol20      float64 `json:"vol20"`
	VolPct     float64 `json:"volPercentile"` // 当前 20 日波动率在过去一年里的分位
	FromHigh   float64 `json:"fromHigh"`      // 距 52 周高点的跌幅
	FromLow    float64 `json:"fromLow"`       // 距 52 周低点的涨幅
	DailyVol   float64 `json:"dailyVol"`
	Bars       int     `json:"bars"`
	LatestDate string  `json:"latestDate"`
}

func TakeSnapshot(dates []string, closes []float64) Snapshot {
	rets := Returns(closes)
	s := Snapshot{
		Price: closes[len(closes)-1], Change1D: Change(closes, 1), Change5D: Change(closes, 5),
		Change20D: Change(closes, 20), Change60D: Change(closes, 60),
		SMA20: SMA(closes, 20), SMA50: SMA(closes, 50), SMA200: SMA(closes, 200),
		RSI14: RSI(closes, 14), Bollinger: Bollinger(closes, 20),
		Vol20: AnnualVol(rets, 20), Bars: len(closes), LatestDate: dates[len(dates)-1],
	}
	s.MACDDif, _, s.MACDHist = MACD(closes)
	s.DailyVol = s.Vol20 / math.Sqrt(252)

	year := closes[max(0, len(closes)-252):]
	s.FromHigh = s.Price/slices.Max(year) - 1
	s.FromLow = s.Price/slices.Min(year) - 1

	// 过去一年每天的 20 日波动率，用来判断现在的波动处在什么水平
	var vols []float64
	for i := max(20, len(rets)-252); i <= len(rets); i++ {
		vols = append(vols, StdDev(rets[i-20:i]))
	}
	s.VolPct = Percentile(vols, s.DailyVol)

	// 数据不足时指标是 NaN，JSON 编码不了，统一置 0
	for _, p := range []*float64{&s.Change1D, &s.Change5D, &s.Change20D, &s.Change60D, &s.SMA20, &s.SMA50, &s.SMA200,
		&s.RSI14, &s.MACDHist, &s.MACDDif, &s.Bollinger, &s.Vol20, &s.VolPct, &s.DailyVol} {
		*p = Finite(*p)
	}
	return s
}

// Finite 把 NaN 和 ±Inf 换成 0。
func Finite(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}
