// Package agents 仿照 TradingAgents（TauricResearch/TradingAgents）的分工做多智能体研判：
//
//	分析师团队（技术 / 宏观 / 情绪与风险）→ 多空研究员辩论 → 交易员给出方案 → 风控经理定仓位和止损
//
// 配置了 Claude 时每个角色由模型扮演，输入是后端算好的真实指标；没配置时用同一套指标走规则引擎，
// 两种模式输出同样的结构，前端不需要区分。
package agents

import (
	"context"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/quant"
)

// MacroMove 是某个宏观因子最近的变化，以及按回归 beta 推算的对该资产的影响。
type MacroMove struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // "return" | "bp"
	Move1D   float64 `json:"move1d"`
	Move5D   float64 `json:"move5d"`
	Beta     float64 `json:"beta"`
	Impact5D float64 `json:"impact5d"` // beta × 5 日变化
}

type Input struct {
	Symbol      string             `json:"symbol"`
	Name        string             `json:"name"`
	Snapshot    quant.Snapshot     `json:"technical"`
	Attribution *quant.Attribution `json:"attribution,omitempty"`
	Macro       []MacroMove        `json:"macro"`
	VIX         float64            `json:"vix"`
	VIXChange5D float64            `json:"vixChange5d"`
	Backtests   []BacktestBrief    `json:"backtests"`
}

// BacktestBrief 是给智能体看的回测摘要，帮助判断哪类策略在这个品种上有效。
type BacktestBrief struct {
	Name     string  `json:"name"`
	CAGR     float64 `json:"cagr"`
	Sharpe   float64 `json:"sharpe"`
	MaxDD    float64 `json:"maxDrawdown"`
	Position float64 `json:"position"`
}

type AnalystReport struct {
	Role    string   `json:"role"`
	Stance  string   `json:"stance"` // 看多 | 中性 | 看空
	Score   float64  `json:"score"`  // -1 ~ 1
	Summary string   `json:"summary"`
	Points  []string `json:"points"`
}

type Argument struct {
	Summary string   `json:"summary"`
	Points  []string `json:"points"`
}

type TraderPlan struct {
	Decision   string  `json:"decision"` // 买入 | 增持 | 持有 | 减持 | 卖出
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type RiskPlan struct {
	Position   float64  `json:"position"` // 建议仓位占组合比例，0~1
	StopLoss   float64  `json:"stopLoss"` // 止损价
	TakeProfit float64  `json:"takeProfit"`
	Notes      []string `json:"notes"`
}

type Final struct {
	Decision   string  `json:"decision"`
	Confidence float64 `json:"confidence"`
	Summary    string  `json:"summary"`
}

type Report struct {
	Symbol      string          `json:"symbol"`
	Name        string          `json:"name"`
	Date        string          `json:"date"`
	Mode        string          `json:"mode"` // "llm" | "rule"
	Model       string          `json:"model,omitempty"`
	Note        string          `json:"note,omitempty"` // 例如模型调用失败、退回规则引擎的原因
	Price       float64         `json:"price"`
	Analysts    []AnalystReport `json:"analysts"`
	Bull        Argument        `json:"bull"`
	Bear        Argument        `json:"bear"`
	Trader      TraderPlan      `json:"trader"`
	Risk        RiskPlan        `json:"risk"`
	Final       Final           `json:"final"`
	GeneratedAt time.Time       `json:"generatedAt"`
}

// Runner 跑一次完整的研判流程。
type Runner interface {
	Run(ctx context.Context, in Input) (Report, error)
}

func stance(score float64) string {
	switch {
	case score >= 0.2:
		return "看多"
	case score <= -0.2:
		return "看空"
	}
	return "中性"
}

func decision(score float64) string {
	switch {
	case score >= 0.5:
		return "买入"
	case score >= 0.2:
		return "增持"
	case score <= -0.5:
		return "卖出"
	case score <= -0.2:
		return "减持"
	}
	return "持有"
}

// 风控统一按波动率目标定仓位：年化波动 15% 时满仓，波动越大仓位越低。
const volTarget = 0.15

func riskPlan(in Input, dec string) RiskPlan {
	s := in.Snapshot
	pos := 1.0
	if s.Vol20 > 0 {
		pos = min(1, volTarget/s.Vol20)
	}
	switch dec {
	case "持有":
		pos *= 0.5
	case "减持":
		pos *= 0.25
	case "卖出":
		pos = 0
	}
	// 止损放在 2 倍日波动 × √10 处，大约是 10 个交易日的 2 个标准差
	band := s.Price * s.DailyVol * 2 * 3.1623
	return RiskPlan{Position: round2(pos), StopLoss: round2(s.Price - band), TakeProfit: round2(s.Price + 1.5*band)}
}

