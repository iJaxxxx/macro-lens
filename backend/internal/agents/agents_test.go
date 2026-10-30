package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/iJaxxxx/macro-lens/backend/internal/llm"
	"github.com/iJaxxxx/macro-lens/backend/internal/quant"
)

func sampleInput() Input {
	return Input{
		Symbol: "VOO", Name: "VOO",
		Snapshot: quant.Snapshot{Price: 500, SMA20: 490, SMA50: 480, SMA200: 450, RSI14: 75, MACDHist: 1, Change20D: 0.03,
			Vol20: 0.2, DailyVol: 0.0126, VolPct: 0.5, FromHigh: -0.01, LatestDate: "2026-10-29"},
		Macro: []MacroMove{{Name: "美元指数 DXY", Kind: "return", Move5D: 0.01, Beta: -0.3, Impact5D: -0.003}},
		VIX:   14,
	}
}

func TestRuleRunner(t *testing.T) {
	rep, err := RuleRunner{}.Run(context.Background(), sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Analysts) != 3 || rep.Mode != "rule" {
		t.Fatalf("report = %+v", rep)
	}
	// 均线多头排列、MACD 为正，技术面应看多；RSI 超买应出现在空方理由里
	if rep.Analysts[0].Stance != "看多" {
		t.Errorf("technical = %+v", rep.Analysts[0])
	}
	if !strings.Contains(strings.Join(rep.Bear.Points, ""), "超买") {
		t.Errorf("bear points = %v", rep.Bear.Points)
	}
	if rep.Risk.Position > 0.75 || rep.Risk.StopLoss >= 500 || rep.Risk.TakeProfit <= 500 {
		t.Errorf("risk = %+v", rep.Risk)
	}
}

// 假的 Messages API：按 system 里的角色返回对应的 JSON，并记录请求体。
func TestLLMRunner(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()

		system := body["system"].([]any)[0].(map[string]any)["text"].(string)
		var out string
		switch {
		case strings.Contains(system, "风控经理"):
			out = `{"decision":"增持","confidence":0.7,"position":1.5,"stopLoss":470,"takeProfit":540,"notes":["注意波动"],"summary":"小幅增持"}`
		case strings.Contains(system, "交易员"):
			out = `{"decision":"买入","confidence":0.8,"rationale":"多方更有说服力"}`
		case strings.Contains(system, "研究员"):
			out = `{"summary":"观点","points":["a","b"]}`
		default:
			out = `{"stance":"看多","score":0.5,"summary":"偏强","points":["均线多头"]}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": body["model"],
			"content":     []map[string]any{{"type": "text", "text": out}},
			"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	client := llm.New("claude-opus-5-5", option.WithBaseURL(srv.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
	rep, err := LLMRunner{Client: client}.Run(context.Background(), sampleInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 7 {
		t.Errorf("calls = %d, want 7 (3 analysts + bull + bear + trader + risk)", len(bodies))
	}
	if rep.Mode != "llm" || rep.Analysts[1].Role != "宏观分析师" || rep.Analysts[1].Stance != "看多" {
		t.Errorf("analysts = %+v", rep.Analysts)
	}
	if rep.Trader.Decision != "买入" || rep.Final.Decision != "增持" || rep.Risk.Position != 1 {
		t.Errorf("trader = %+v final = %+v risk = %+v", rep.Trader, rep.Final, rep.Risk)
	}

	b := bodies[0]
	oc, _ := b["output_config"].(map[string]any)
	if b["fallbacks"] != "default" || oc["format"] == nil || oc["effort"] == nil {
		t.Errorf("request body = %v", b)
	}
}
