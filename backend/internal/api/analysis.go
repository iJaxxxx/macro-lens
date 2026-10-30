package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/agents"
	"github.com/iJaxxxx/macro-lens/backend/internal/market"
	"github.com/iJaxxxx/macro-lens/backend/internal/quant"
)

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func closesOf(bars []market.Bar) ([]string, []float64) {
	dates, closes := make([]string, len(bars)), make([]float64, len(bars))
	for i, b := range bars {
		dates[i], closes[i] = b.Date, b.Close
	}
	return dates, closes
}

// 回测统一用最近三年（约 756 个交易日），各策略起点相同才可比
const backtestDays = 756

// runBacktests 返回各策略结果和回测起点的下标
func runBacktests(closes []float64) ([]quant.BacktestResult, int) {
	start := max(200, len(closes)-1-backtestDays)
	out := make([]quant.BacktestResult, len(quant.Strategies))
	for i, s := range quant.Strategies {
		out[i] = quant.Backtest(s, closes, start)
	}
	return out, start
}

type BacktestView struct {
	Symbol     string                 `json:"symbol"`
	Name       string                 `json:"name"`
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	Strategies []quant.BacktestResult `json:"strategies"`
}

func computeBacktest(ctx context.Context, m *marketCache, t tracked) (BacktestView, error) {
	bars, err := m.liveHistory(ctx, t.symbol)
	if err != nil {
		return BacktestView{}, err
	}
	if len(bars) < 260 {
		return BacktestView{}, fmt.Errorf("%s 的历史数据不足", t.symbol)
	}
	dates, closes := closesOf(bars)
	results, start := runBacktests(closes)
	return BacktestView{Symbol: t.symbol, Name: t.sample.Name, From: dates[start], To: dates[len(dates)-1], Strategies: results}, nil
}

// buildInput 汇总智能体需要的全部数据：技术面、宏观因子、归因、回测和 VIX。
func buildInput(ctx context.Context, m *marketCache, t tracked) (agents.Input, error) {
	attr, err := computeAttribution(ctx, m, t)
	if err != nil {
		return agents.Input{}, err
	}
	bars, err := m.histories(ctx, append([]string{t.symbol, "^VIX"}, t.factors...)...)
	if err != nil {
		return agents.Input{}, err
	}
	in := agents.Input{Symbol: t.symbol, Name: t.sample.Name, Snapshot: attr.Snapshot, Attribution: &quant.Attribution{
		Date: attr.Date, Total: attr.Total, R2: attr.R2, Window: attr.Window,
	}}
	for _, r := range attr.Rows {
		in.Attribution.Contributions = append(in.Attribution.Contributions, quant.Contribution{Name: r.Name, Beta: r.Beta, Contribution: r.Contribution, Share: r.Share})
	}

	betas := map[string]float64{}
	for _, r := range attr.Rows {
		betas[r.Name] = r.Beta
	}
	for _, sym := range t.factors {
		f := findFactor(sym)
		kind := "return"
		if f.kind == quant.FactorDiffBP {
			kind = "bp"
		}
		mm := agents.MacroMove{Name: f.tile, Kind: kind, Beta: betas[f.name],
			Move1D: quant.Finite(factorMove(f.kind, bars[sym], 1)), Move5D: quant.Finite(factorMove(f.kind, bars[sym], 5))}
		mm.Impact5D = mm.Beta * mm.Move5D
		in.Macro = append(in.Macro, mm)
	}
	if vix := bars["^VIX"]; len(vix) > 5 {
		in.VIX = vix[len(vix)-1].Close
		in.VIXChange5D = quant.Finite(factorMove(quant.FactorReturn, vix, 5))
	}

	_, closes := closesOf(bars[t.symbol])
	if len(closes) >= 260 {
		results, _ := runBacktests(closes)
		for _, r := range results {
			in.Backtests = append(in.Backtests, agents.BacktestBrief{Name: r.Name, CAGR: r.CAGR, Sharpe: r.Sharpe, MaxDD: r.MaxDrawdown, Position: r.Position})
		}
	}
	return in, nil
}

// analysisService 缓存研判结果。大模型版一次要调 7 次模型，按品种缓存 6 小时；
// 规则版很便宜，5 分钟刷新一次。同一品种同时只跑一个。
type analysisService struct {
	market *marketCache
	llm    agents.Runner // nil 表示没配置大模型
	rule   agents.Runner

	mu      sync.Mutex
	entries map[string]*analysisEntry
}

type analysisEntry struct {
	mu         sync.Mutex
	rep        agents.Report
	at         time.Time
	ttl        time.Duration
	refreshing bool
}

const (
	llmAnalysisTTL  = 6 * time.Hour
	ruleAnalysisTTL = 5 * time.Minute
	// 大模型调用失败后用规则结果顶上，过一段时间再重试，避免持续失败时反复调用
	llmRetryTTL = 30 * time.Minute
)

// get 第一次请求某个品种时同步计算；之后过期了先返回旧结果，在后台刷新。
func (s *analysisService) get(ctx context.Context, t tracked) (agents.Report, error) {
	s.mu.Lock()
	e, ok := s.entries[t.symbol]
	if !ok {
		e = &analysisEntry{}
		s.entries[t.symbol] = e
	}
	s.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.at.IsZero() {
		if time.Since(e.at) >= e.ttl && !e.refreshing {
			e.refreshing = true
			go func() {
				rep, ttl, err := s.compute(context.Background(), t)
				e.mu.Lock()
				defer e.mu.Unlock()
				e.refreshing = false
				if err == nil {
					e.rep, e.at, e.ttl = rep, time.Now(), ttl
				}
			}()
		}
		return e.rep, nil
	}
	rep, ttl, err := s.compute(context.WithoutCancel(ctx), t)
	if err != nil {
		return agents.Report{}, err
	}
	e.rep, e.at, e.ttl = rep, time.Now(), ttl
	return rep, nil
}

// compute 返回研判结果和它的缓存时长
func (s *analysisService) compute(ctx context.Context, t tracked) (agents.Report, time.Duration, error) {
	in, err := buildInput(ctx, s.market, t)
	if err != nil {
		return agents.Report{}, 0, err
	}
	if s.llm != nil {
		lctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		defer cancel()
		rep, err := s.llm.Run(lctx, in)
		if err == nil {
			return rep, llmAnalysisTTL, nil
		}
		log.Printf("analysis %s: llm failed, fallback to rules: %v", t.symbol, err)
		rep, _ = s.rule.Run(ctx, in)
		rep.Note = "大模型调用失败，本次结果由规则引擎生成，稍后会自动重试"
		return rep, llmRetryTTL, nil
	}
	rep, err := s.rule.Run(ctx, in)
	return rep, ruleAnalysisTTL, err
}

type ScoreItem struct {
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Hits   int     `json:"hits"`
	Total  int     `json:"total"`
	Rate   float64 `json:"rate"`
}

type Scorecard struct {
	Days  int         `json:"days"`
	Hits  int         `json:"hits"`
	Total int         `json:"total"`
	Rate  float64     `json:"rate"`
	Items []ScoreItem `json:"items"`
}

// 预测成绩单：技术面综合信号对「次日涨跌方向」的命中率
const scoreDays = 30

func computeScorecard(ctx context.Context, m *marketCache) (Scorecard, error) {
	sc := Scorecard{Days: scoreDays}
	for _, sym := range []string{"^GSPC", "GC=F", "CL=F"} {
		bars, err := m.liveHistory(ctx, sym)
		if err != nil {
			return Scorecard{}, err
		}
		_, closes := closesOf(bars)
		t, _ := findTracked(sym)
		hits, total := quant.HitRate(closes, scoreDays)
		item := ScoreItem{Symbol: sym, Name: t.sample.Name, Hits: hits, Total: total}
		if total > 0 {
			item.Rate = float64(hits) / float64(total)
		}
		sc.Items = append(sc.Items, item)
		sc.Hits += hits
		sc.Total += total
	}
	if sc.Total > 0 {
		sc.Rate = float64(sc.Hits) / float64(sc.Total)
	}
	return sc, nil
}
