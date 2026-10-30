package agents

import (
	"context"
	"fmt"
	"math"
	"time"
)

// RuleRunner 不依赖大模型，按固定规则把指标翻译成各角色的观点。
type RuleRunner struct{}

type signal struct {
	text string
	dir  float64 // +1 利多，-1 利空，0 中性
}

func pct(x float64) string { return fmt.Sprintf("%+.2f%%", x*100) }

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// analyst 的得分是各信号方向的平均值，中性信号也计入分母
func analyst(role string, sigs []signal) AnalystReport {
	var sum float64
	var points []string
	bull, bear := 0, 0
	for _, s := range sigs {
		points = append(points, s.text)
		sum += s.dir
		if s.dir > 0 {
			bull++
		} else if s.dir < 0 {
			bear++
		}
	}
	score := 0.0
	if len(sigs) > 0 {
		score = sum / float64(len(sigs))
	}
	return AnalystReport{Role: role, Score: round2(score), Stance: stance(score), Points: points,
		Summary: fmt.Sprintf("%d 项利多、%d 项利空，综合判断%s。", bull, bear, stance(score))}
}

func technicalSignals(in Input) []signal {
	s := in.Snapshot
	var out []signal
	above := func(name string, ma float64) {
		if ma == 0 {
			return
		}
		d := s.Price/ma - 1
		if d >= 0 {
			out = append(out, signal{fmt.Sprintf("价格高于 %s %s，趋势偏强", name, pct(d)), 1})
		} else {
			out = append(out, signal{fmt.Sprintf("价格低于 %s %s，趋势偏弱", name, pct(d)), -1})
		}
	}
	above("20 日均线", s.SMA20)
	above("50 日均线", s.SMA50)
	above("200 日均线", s.SMA200)
	if s.MACDHist > 0 {
		out = append(out, signal{"MACD 柱线为正，短期动能向上", 1})
	} else {
		out = append(out, signal{"MACD 柱线为负，短期动能向下", -1})
	}
	switch {
	case s.RSI14 > 70:
		out = append(out, signal{fmt.Sprintf("RSI(14) 为 %.0f，处于超买区，有回调风险", s.RSI14), -1})
	case s.RSI14 < 30:
		out = append(out, signal{fmt.Sprintf("RSI(14) 为 %.0f，处于超卖区，存在反弹可能", s.RSI14), 1})
	default:
		out = append(out, signal{fmt.Sprintf("RSI(14) 为 %.0f，处于中性区间", s.RSI14), 0})
	}
	out = append(out, signal{fmt.Sprintf("近 20 个交易日涨跌 %s", pct(s.Change20D)), math.Copysign(1, s.Change20D)})

	// 历史回测里夏普最高的策略当前给出的信号
	var best *BacktestBrief
	for i, b := range in.Backtests {
		if best == nil || b.Sharpe > best.Sharpe {
			best = &in.Backtests[i]
		}
	}
	if best != nil && best.Sharpe > 0 {
		state, dir := "空仓", -1.0
		if best.Position > 0 {
			state, dir = "持有", 1
		}
		out = append(out, signal{fmt.Sprintf("五年回测夏普最高的「%s」（夏普 %.2f）当前信号为%s", best.Name, best.Sharpe, state), dir})
	}
	return out
}

func macroSignals(in Input) []signal {
	var out []signal
	var total float64
	for _, m := range in.Macro {
		total += m.Impact5D
		move := pct(m.Move5D)
		if m.Kind == "bp" {
			move = fmt.Sprintf("%+.0fbp", m.Move5D)
		}
		dir := 0.0
		// 推算影响小于 0.1% 视为可以忽略
		if math.Abs(m.Impact5D) >= 0.001 {
			dir = math.Copysign(1, m.Impact5D)
		}
		out = append(out, signal{fmt.Sprintf("%s 近 5 日 %s，按回归 beta 推算影响 %s", m.Name, move, pct(m.Impact5D)), dir})
	}
	if len(in.Macro) > 0 {
		out = append(out, signal{fmt.Sprintf("宏观因子合计推算影响 %s", pct(total)), sign(total, 0.002)})
	}
	return out
}

func sentimentSignals(in Input) []signal {
	s := in.Snapshot
	var out []signal
	switch {
	case in.VIX >= 25:
		out = append(out, signal{fmt.Sprintf("VIX 为 %.1f，市场处于恐慌状态", in.VIX), -1})
	case in.VIX > 0 && in.VIX < 15:
		out = append(out, signal{fmt.Sprintf("VIX 为 %.1f，风险偏好较高", in.VIX), 1})
	case in.VIX > 0:
		out = append(out, signal{fmt.Sprintf("VIX 为 %.1f，情绪中性", in.VIX), 0})
	}
	if in.VIX > 0 {
		out = append(out, signal{fmt.Sprintf("VIX 近 5 日 %s", pct(in.VIXChange5D)), -sign(in.VIXChange5D, 0.05)})
	}
	switch {
	case s.VolPct >= 0.8:
		out = append(out, signal{fmt.Sprintf("20 日年化波动 %.1f%%，处于过去一年的高位（%.0f 分位）", s.Vol20*100, s.VolPct*100), -1})
	case s.VolPct <= 0.2:
		out = append(out, signal{fmt.Sprintf("20 日年化波动 %.1f%%，处于过去一年的低位（%.0f 分位）", s.Vol20*100, s.VolPct*100), 1})
	default:
		out = append(out, signal{fmt.Sprintf("20 日年化波动 %.1f%%（%.0f 分位）", s.Vol20*100, s.VolPct*100), 0})
	}
	if s.FromHigh <= -0.1 {
		out = append(out, signal{fmt.Sprintf("距 52 周高点回撤 %s，处于调整阶段", pct(s.FromHigh)), -1})
	} else {
		out = append(out, signal{fmt.Sprintf("距 52 周高点 %s", pct(s.FromHigh)), 1})
	}
	return out
}

func sign(x, deadband float64) float64 {
	switch {
	case x > deadband:
		return 1
	case x < -deadband:
		return -1
	}
	return 0
}

// 三位分析师的权重
var weights = map[string]float64{"技术分析师": 0.4, "宏观分析师": 0.35, "情绪与风险分析师": 0.25}

func (RuleRunner) Run(_ context.Context, in Input) (Report, error) {
	rep := Report{Symbol: in.Symbol, Name: in.Name, Date: in.Snapshot.LatestDate, Mode: "rule", Price: in.Snapshot.Price, GeneratedAt: time.Now()}

	var all []signal
	var score float64
	for _, a := range []struct {
		role string
		sigs []signal
	}{
		{"技术分析师", technicalSignals(in)},
		{"宏观分析师", macroSignals(in)},
		{"情绪与风险分析师", sentimentSignals(in)},
	} {
		r := analyst(a.role, a.sigs)
		rep.Analysts = append(rep.Analysts, r)
		all = append(all, a.sigs...)
		score += weights[a.role] * r.Score
	}

	for _, s := range all {
		if s.dir > 0 {
			rep.Bull.Points = append(rep.Bull.Points, s.text)
		} else if s.dir < 0 {
			rep.Bear.Points = append(rep.Bear.Points, s.text)
		}
	}
	rep.Bull.Summary = fmt.Sprintf("多方列出 %d 条理由。", len(rep.Bull.Points))
	rep.Bear.Summary = fmt.Sprintf("空方列出 %d 条理由。", len(rep.Bear.Points))

	dec := decision(score)
	conf := round2(0.5 + 0.5*math.Min(1, math.Abs(score)))
	rep.Trader = TraderPlan{Decision: dec, Confidence: conf,
		Rationale: fmt.Sprintf("按 技术 40%% / 宏观 35%% / 情绪 25%% 加权，综合得分 %+.2f，对应「%s」。", score, dec)}

	rep.Risk = riskPlan(in, dec)
	rep.Risk.Notes = []string{
		fmt.Sprintf("按年化波动 %.0f%% 的目标定仓位，当前 20 日波动 %.1f%%", volTarget*100, in.Snapshot.Vol20*100),
		"止损约为 10 个交易日 2 倍标准差的波动区间",
	}
	rep.Final = Final{Decision: dec, Confidence: conf,
		Summary: fmt.Sprintf("%s：%s，建议仓位 %.0f%%，止损 %.2f。", in.Name, dec, rep.Risk.Position*100, rep.Risk.StopLoss)}
	return rep, nil
}
