package api

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iJaxxxx/macro-lens/backend/internal/llm"
	"github.com/iJaxxxx/macro-lens/backend/internal/quant"
)

// 回归窗口：约半年的交易日，既能反映当前的因子关系，又有足够样本
const attributionWindow = 120

type AttributionRow struct {
	Name         string  `json:"name"`
	Factor       string  `json:"factor"`
	Move         string  `json:"move"`
	Beta         float64 `json:"beta"`
	Contribution float64 `json:"contribution"`
	Share        float64 `json:"share"`
}

type AttributionView struct {
	Symbol   string           `json:"symbol"`
	Name     string           `json:"name"`
	Date     string           `json:"date"`
	Total    float64          `json:"total"`
	R2       float64          `json:"r2"`
	Window   int              `json:"window"`
	Rows     []AttributionRow `json:"rows"` // 按贡献绝对值从大到小，最后一行是「其他」
	Title    string           `json:"title"`
	Summary  string           `json:"summary"`
	Source   string           `json:"source"` // "rule" | "llm"
	Snapshot quant.Snapshot   `json:"technical"`
}

func computeAttribution(ctx context.Context, m *marketCache, t tracked) (AttributionView, error) {
	bars, err := m.histories(ctx, append([]string{t.symbol}, t.factors...)...)
	if err != nil {
		return AttributionView{}, err
	}
	inputs := make([]quant.FactorInput, len(t.factors))
	for i, sym := range t.factors {
		f := findFactor(sym)
		inputs[i] = quant.FactorInput{Name: f.name, Kind: f.kind, Bars: bars[sym]}
	}
	a, err := quant.Attribute(bars[t.symbol], inputs, attributionWindow)
	if err != nil {
		return AttributionView{}, err
	}

	v := AttributionView{Symbol: t.symbol, Name: t.sample.Name, Date: a.Date, Total: a.Total, R2: a.R2, Window: a.Window, Source: "rule"}
	for i, c := range a.Contributions {
		f := findFactor(t.factors[i])
		v.Rows = append(v.Rows, AttributionRow{Name: c.Name, Factor: f.tile, Move: describeMove(f.kind, c.Move), Beta: c.Beta, Contribution: c.Contribution, Share: c.Share})
	}
	slices.SortFunc(v.Rows, func(a, b AttributionRow) int { return cmp.Compare(math.Abs(b.Contribution), math.Abs(a.Contribution)) })
	v.Rows = append(v.Rows, AttributionRow{Name: "其他", Factor: "因子以外的特异因素", Contribution: a.Residual, Share: a.ResidualShare})

	closes := make([]float64, len(bars[t.symbol]))
	dates := make([]string, len(closes))
	for i, b := range bars[t.symbol] {
		closes[i], dates[i] = b.Close, b.Date
	}
	v.Snapshot = quant.TakeSnapshot(dates, closes)
	v.Title, v.Summary = ruleNarrative(v)
	return v, nil
}

func signedPct(x float64) string { return fmt.Sprintf("%+.2f%%", x*100) }

// ruleNarrative 按归因结果拼出解读文字，所有数字都来自计算结果。
func ruleNarrative(v AttributionView) (string, string) {
	verb := "涨"
	if v.Total < 0 {
		verb = "跌"
	}
	title := fmt.Sprintf("为什么%s今天%s了？", v.Name, verb)

	var b strings.Builder
	fmt.Fprintf(&b, "%s 最新交易日（%s）%s %.2f%%。", v.Name, v.Date, verb, math.Abs(v.Total)*100)
	factors := v.Rows[:len(v.Rows)-1]
	other := v.Rows[len(v.Rows)-1]
	var drivers, against []string
	for _, r := range factors {
		if math.Abs(r.Contribution) < 0.0002 {
			continue
		}
		s := fmt.Sprintf("%s（%s %s，贡献 %s）", r.Name, r.Factor, r.Move, signedPct(r.Contribution))
		if (r.Contribution > 0) == (v.Total > 0) {
			drivers = append(drivers, s)
		} else {
			against = append(against, s)
		}
	}
	if len(drivers) > 0 {
		fmt.Fprintf(&b, "主要推动力是%s。", strings.Join(drivers[:min(2, len(drivers))], "，以及"))
	}
	if len(against) > 0 {
		fmt.Fprintf(&b, "%s形成反向拖累。", against[0])
	}
	if other.Share >= 0.5 {
		fmt.Fprintf(&b, "不过宏观因子只能解释一小部分，%s 来自因子以外的特异因素（如行业消息、供需或资金流）。", signedPct(other.Contribution))
	} else {
		fmt.Fprintf(&b, "因子之外的特异因素贡献 %s。", signedPct(other.Contribution))
	}
	return title, b.String()
}

// narrator 在后台用 Claude 改写归因解读，结果按「品种 + 交易日」缓存一小时。
// 第一次请求先返回规则版文字，不让用户等模型。
type narrator struct {
	client *llm.Client

	mu      sync.Mutex
	entries map[string]*narration
}

type narration struct {
	text    string
	at      time.Time
	running bool
}

const narrationTTL = time.Hour

var narrationSchema = llm.Object(map[string]any{
	"summary": llm.String("三到四句话的中文解读，给普通投资者看"),
})

func (n *narrator) get(v AttributionView) (string, bool) {
	if n == nil || n.client == nil {
		return "", false
	}
	key := v.Symbol + "|" + v.Date
	n.mu.Lock()
	defer n.mu.Unlock()
	e, ok := n.entries[key]
	if !ok {
		e = &narration{}
		n.entries[key] = e
	}
	if !e.running && time.Since(e.at) > narrationTTL {
		e.running = true
		go n.generate(e, v)
	}
	return e.text, e.text != ""
}

func (n *narrator) generate(e *narration, v AttributionView) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	data := toJSON(map[string]any{"asset": v.Name, "date": v.Date, "total": v.Total, "r2": v.R2, "window": v.Window, "rows": v.Rows,
		"technical": map[string]any{"change5d": v.Snapshot.Change5D, "change20d": v.Snapshot.Change20D, "rsi14": v.Snapshot.RSI14}})
	var out struct {
		Summary string `json:"summary"`
	}
	err := n.client.AskJSON(ctx,
		"你是宏观策略分析师。根据多因子回归的涨跌归因结果，用简体中文解释该资产今天为什么涨或跌。"+
			"只依据给出的数据，不要编造新闻；百分比字段是小数（0.012 表示 1.2%）。解释主要驱动因子背后的经济逻辑，并提示 R² 代表的解释力度。",
		"归因数据：\n"+data, narrationSchema, llm.EffortLow, &out)

	n.mu.Lock()
	defer n.mu.Unlock()
	e.running = false
	e.at = time.Now()
	if err != nil {
		log.Printf("narration %s: %v", v.Symbol, err)
		// 失败后过 10 分钟再试
		e.at = time.Now().Add(-narrationTTL + 10*time.Minute)
		return
	}
	e.text = out.Summary
}
