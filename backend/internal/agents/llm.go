package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/iJaxxxx/macro-lens/backend/internal/llm"
)

// LLMRunner 让 Claude 依次扮演各个角色。每个角色只看到它该看的数据和上游角色的结论。
type LLMRunner struct {
	Client *llm.Client
}

const common = `你是一家宏观对冲基金投研团队中的一员，团队采用多智能体分工协作。
只能依据用户消息里提供的数据做判断，不要编造新闻、财报或数据里没有的数字。
所有百分比字段在数据中以小数表示（0.012 表示 1.2%），"bp" 表示基点。用简体中文回答，表述简洁专业。`

var analystSchema = llm.Object(map[string]any{
	"stance":  llm.Enum("对该资产未来 1~4 周的观点", "看多", "中性", "看空"),
	"score":   llm.Number("观点强度，-1 为强烈看空，1 为强烈看多"),
	"summary": llm.String("一到两句话的结论"),
	"points":  llm.Strings("3~5 条论据，每条引用具体数字"),
})

var argumentSchema = llm.Object(map[string]any{
	"summary": llm.String("一到两句话概括本方立场"),
	"points":  llm.Strings("3~4 条最有力的论点，可以反驳对方"),
})

var traderSchema = llm.Object(map[string]any{
	"decision":   llm.Enum("交易决策", "买入", "增持", "持有", "减持", "卖出"),
	"confidence": llm.Number("置信度，0.5~1"),
	"rationale":  llm.String("两到三句话说明为什么这样决策，以及多空哪一方更有说服力"),
})

var riskSchema = llm.Object(map[string]any{
	"decision":   llm.Enum("风控审核后的最终决策，可以比交易员更保守", "买入", "增持", "持有", "减持", "卖出"),
	"confidence": llm.Number("最终置信度，0.5~1"),
	"position":   llm.Number("建议仓位占组合比例，0~1"),
	"stopLoss":   llm.Number("止损价"),
	"takeProfit": llm.Number("止盈价"),
	"notes":      llm.Strings("2~3 条风险提示"),
	"summary":    llm.String("一句话的最终结论，给普通投资者看"),
})

var roles = []struct {
	name   string
	prompt string
	data   func(Input) any
}{
	{"技术分析师", "你是技术分析师。根据价格、均线、MACD、RSI、布林带、波动率以及各策略的历史回测，判断趋势和动能。",
		func(in Input) any { return map[string]any{"technical": in.Snapshot, "backtests": in.Backtests} }},
	{"宏观分析师", "你是宏观分析师。根据各宏观因子近期变化、它们对该资产的回归 beta 以及今日涨跌归因，判断宏观环境对该资产是顺风还是逆风。",
		func(in Input) any { return map[string]any{"macro": in.Macro, "attribution": in.Attribution} }},
	{"情绪与风险分析师", "你是情绪与风险分析师。根据 VIX、波动率分位、距 52 周高低点的位置，判断市场情绪和下行风险。",
		func(in Input) any {
			s := in.Snapshot
			return map[string]any{"vix": in.VIX, "vixChange5d": in.VIXChange5D, "vol20": s.Vol20, "volPercentile": s.VolPct,
				"fromHigh": s.FromHigh, "fromLow": s.FromLow, "change5d": s.Change5D, "change20d": s.Change20D}
		}},
}

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (r LLMRunner) Run(ctx context.Context, in Input) (Report, error) {
	rep := Report{Symbol: in.Symbol, Name: in.Name, Date: in.Snapshot.LatestDate, Mode: "llm", Model: r.Client.Model(), Price: in.Snapshot.Price}
	head := fmt.Sprintf("标的：%s（%s），数据截至 %s，最新价 %.2f。\n\n", in.Name, in.Symbol, in.Snapshot.LatestDate, in.Snapshot.Price)

	// 第一步：三位分析师并行出报告
	rep.Analysts = make([]AnalystReport, len(roles))
	g, gctx := errgroup.WithContext(ctx)
	for i, role := range roles {
		g.Go(func() error {
			a := &rep.Analysts[i]
			a.Role = role.name
			return r.Client.AskJSON(gctx, common+"\n\n"+role.prompt, head+"数据：\n"+toJSON(role.data(in)), analystSchema, llm.EffortLow, a)
		})
	}
	if err := g.Wait(); err != nil {
		return Report{}, err
	}
	reports := head + "分析师报告：\n" + toJSON(rep.Analysts)

	// 第二步：多空研究员各自立论
	g, gctx = errgroup.WithContext(ctx)
	g.Go(func() error {
		return r.Client.AskJSON(gctx, common+"\n\n你是多方研究员，任务是从分析师报告中找出支持做多的最强理由，并回应可能的看空观点。",
			reports, argumentSchema, llm.EffortLow, &rep.Bull)
	})
	g.Go(func() error {
		return r.Client.AskJSON(gctx, common+"\n\n你是空方研究员，任务是从分析师报告中找出支持看空或回避的最强理由，并指出多方逻辑的漏洞。",
			reports, argumentSchema, llm.EffortLow, &rep.Bear)
	})
	if err := g.Wait(); err != nil {
		return Report{}, err
	}
	debate := reports + "\n\n多方观点：\n" + toJSON(rep.Bull) + "\n\n空方观点：\n" + toJSON(rep.Bear)

	// 第三步：交易员权衡多空，给出方案
	if err := r.Client.AskJSON(ctx, common+"\n\n你是交易员。权衡分析师报告和多空辩论，给出对该资产的交易决策。",
		debate, traderSchema, llm.EffortMedium, &rep.Trader); err != nil {
		return Report{}, err
	}

	// 第四步：风控经理审核并定仓位
	var risk struct {
		RiskPlan
		Decision   string  `json:"decision"`
		Confidence float64 `json:"confidence"`
		Summary    string  `json:"summary"`
	}
	ref := riskPlan(in, rep.Trader.Decision)
	riskInput := debate + "\n\n交易员方案：\n" + toJSON(rep.Trader) + fmt.Sprintf(
		"\n\n参考数据：20 日年化波动 %.4f，日波动 %.4f；按年化 15%% 波动目标推算的参考仓位 %.2f，参考止损 %.2f，参考止盈 %.2f。",
		in.Snapshot.Vol20, in.Snapshot.DailyVol, ref.Position, ref.StopLoss, ref.TakeProfit)
	if err := r.Client.AskJSON(ctx, common+"\n\n你是风控经理。审核交易员方案，结合波动率给出仓位、止损、止盈和最终决策。仓位必须在 0~1 之间，止损价必须低于最新价。",
		riskInput, riskSchema, llm.EffortMedium, &risk); err != nil {
		return Report{}, err
	}
	rep.Risk = risk.RiskPlan
	rep.Risk.Position = math.Max(0, math.Min(1, rep.Risk.Position))
	rep.Final = Final{Decision: risk.Decision, Confidence: risk.Confidence, Summary: risk.Summary}
	rep.GeneratedAt = time.Now()
	return rep, nil
}
