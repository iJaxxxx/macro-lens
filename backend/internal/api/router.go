package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/iJaxxxx/macro-lens/backend/internal/agents"
	"github.com/iJaxxxx/macro-lens/backend/internal/auth"
	"github.com/iJaxxxx/macro-lens/backend/internal/llm"
	"github.com/iJaxxxx/macro-lens/backend/internal/portfolio"
)

type Config struct {
	StaticDir      string     // 非空时同时托管前端打包产物（frontend/dist）
	Market         MarketData // nil 时行情接口只返回示例数据，归因等接口返回 503
	LLM            *llm.Client
	GoogleClientID string
	AllowedEmails  []string
	SessionSecret  []byte
	DataDir        string
}

// NewRouter 返回整个站点的路由。
func NewRouter(cfg Config) http.Handler {
	m := newMarketCache(cfg.Market)
	analysis := &analysisService{market: m, rule: agents.RuleRunner{}, entries: map[string]*analysisEntry{}}
	if cfg.LLM != nil {
		analysis.llm = agents.LLMRunner{Client: cfg.LLM}
	}
	narr := &narrator{client: cfg.LLM, entries: map[string]*narration{}}
	acct := &accountHandlers{sessions: auth.NewSessions(cfg.SessionSecret), store: portfolio.NewStore(cfg.DataDir), market: m}
	if cfg.GoogleClientID != "" {
		acct.verifier = auth.NewVerifier(cfg.GoogleClientID)
	}
	for _, e := range cfg.AllowedEmails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			acct.allowed = append(acct.allowed, e)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		c := map[string]any{"googleClientId": cfg.GoogleClientID, "llm": cfg.LLM != nil}
		if cfg.LLM != nil {
			c["model"] = cfg.LLM.Model()
		}
		writeJSON(w, c)
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listAssets(r.Context(), m))
	})
	mux.HandleFunc("GET /api/factors", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listFactors(r.Context(), m))
	})
	mux.HandleFunc("GET /api/attribution", withTracked(m, func(w http.ResponseWriter, r *http.Request, t tracked) {
		v, err := computeAttribution(r.Context(), m, t)
		if err != nil {
			unavailable(w, err)
			return
		}
		if text, ok := narr.get(v); ok {
			v.Summary, v.Source = text, "llm"
		}
		writeJSON(w, v)
	}))
	mux.HandleFunc("GET /api/analysis", withTracked(m, func(w http.ResponseWriter, r *http.Request, t tracked) {
		rep, err := analysis.get(r.Context(), t)
		if err != nil {
			unavailable(w, err)
			return
		}
		writeJSON(w, rep)
	}))
	mux.HandleFunc("GET /api/backtest", withTracked(m, func(w http.ResponseWriter, r *http.Request, t tracked) {
		v, err := computeBacktest(r.Context(), m, t)
		if err != nil {
			unavailable(w, err)
			return
		}
		writeJSON(w, v)
	}))
	mux.HandleFunc("GET /api/scorecard", func(w http.ResponseWriter, r *http.Request) {
		if m == nil {
			writeError(w, http.StatusServiceUnavailable, "未接入行情源")
			return
		}
		sc, err := computeScorecard(r.Context(), m)
		if err != nil {
			unavailable(w, err)
			return
		}
		writeJSON(w, sc)
	})

	mux.HandleFunc("POST /api/auth/google", acct.login)
	mux.HandleFunc("POST /api/auth/logout", acct.logout)
	mux.HandleFunc("GET /api/me", acct.me)
	mux.HandleFunc("GET /api/portfolio", acct.requireUser(acct.getPortfolio))
	mux.HandleFunc("POST /api/portfolio/transactions", acct.requireUser(acct.addTransaction))
	mux.HandleFunc("DELETE /api/portfolio/transactions/{id}", acct.requireUser(acct.deleteTransaction))

	if cfg.StaticDir != "" {
		// "/" 匹配所有没被上面 /api 路由接住的请求
		mux.Handle("GET /", spaHandler(cfg.StaticDir))
	}
	return mux
}

// withTracked 从 ?symbol= 取出要分析的品种，只接受首页跟踪的那几个，避免被当成任意行情代理。
func withTracked(m *marketCache, next func(http.ResponseWriter, *http.Request, tracked)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, ok := findTracked(r.URL.Query().Get("symbol"))
		if !ok {
			writeError(w, http.StatusBadRequest, "不支持的品种")
			return
		}
		if m == nil {
			writeError(w, http.StatusServiceUnavailable, "未接入行情源")
			return
		}
		next(w, r, t)
	}
}

func unavailable(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	writeError(w, http.StatusServiceUnavailable, "行情数据暂时不可用，请稍后重试")
}

// spaHandler 托管前端静态文件：文件存在就直接返回，不存在就返回 index.html，
// 交给前端自己处理页面路由。
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("api: encode response: %v", err)
		writeError(w, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(append(b, '\n'))
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
