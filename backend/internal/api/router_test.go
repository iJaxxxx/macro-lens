package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iJaxxxx/macro-lens/backend/internal/agents"
	"github.com/iJaxxxx/macro-lens/backend/internal/auth"
)

func testConfig(t *testing.T) Config {
	return Config{Market: &fakeMarket{}, SessionSecret: []byte("test"), DataDir: t.TempDir()}
}

func do(t *testing.T, h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestHealth(t *testing.T) {
	if rec := do(t, NewRouter(Config{}), "GET", "/api/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAssetsWithoutMarket(t *testing.T) {
	got := decode[[]Asset](t, do(t, NewRouter(Config{}), "GET", "/api/assets", ""))
	if len(got) != len(trackedAssets) || got[0].Live {
		t.Fatalf("assets = %+v", got)
	}
}

func TestMarketEndpoints(t *testing.T) {
	h := NewRouter(testConfig(t))

	factors := decode[[]FactorTile](t, do(t, h, "GET", "/api/factors", ""))
	if len(factors) != len(factorTiles) || !factors[0].Available {
		t.Errorf("factors = %+v", factors)
	}

	rep := decode[agents.Report](t, do(t, h, "GET", "/api/analysis?symbol=VOO", ""))
	if rep.Mode != "rule" || len(rep.Analysts) != 3 || rep.Final.Decision == "" || !slices.Contains([]string{"买入", "增持", "持有", "减持", "卖出"}, rep.Final.Decision) {
		t.Errorf("analysis = %+v", rep)
	}
	if rep.Risk.StopLoss >= rep.Price || rep.Risk.Position < 0 || rep.Risk.Position > 1 {
		t.Errorf("risk = %+v, price %v", rep.Risk, rep.Price)
	}

	bt := decode[BacktestView](t, do(t, h, "GET", "/api/backtest?symbol=CL%3DF", ""))
	if len(bt.Strategies) != 6 || len(bt.Strategies[0].Equity) == 0 || bt.From >= bt.To {
		t.Errorf("backtest = %+v", bt)
	}

	sc := decode[Scorecard](t, do(t, h, "GET", "/api/scorecard", ""))
	if len(sc.Items) != 3 || sc.Total == 0 {
		t.Errorf("scorecard = %+v", sc)
	}

	if rec := do(t, h, "GET", "/api/attribution?symbol=AAPL", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("untracked symbol status = %d", rec.Code)
	}
	if rec := do(t, NewRouter(Config{}), "GET", "/api/attribution?symbol=VOO", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no market status = %d", rec.Code)
	}
}

func TestPortfolioRequiresLogin(t *testing.T) {
	cfg := testConfig(t)
	h := NewRouter(cfg)
	if rec := do(t, h, "GET", "/api/portfolio", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous status = %d", rec.Code)
	}
	// 没配置 GOOGLE_CLIENT_ID 时登录接口不可用
	if rec := do(t, h, "POST", "/api/auth/google", `{"credential":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("login status = %d", rec.Code)
	}

	// 用同一个密钥签一个会话，模拟已登录
	rec := httptest.NewRecorder()
	auth.NewSessions(cfg.SessionSecret).Issue(rec, httptest.NewRequest("GET", "/", nil), auth.User{Sub: "1", Email: "a@gmail.com"})
	cookie := rec.Result().Cookies()[0]

	if me := decode[auth.User](t, do(t, h, "GET", "/api/me", "", cookie)); me.Email != "a@gmail.com" {
		t.Errorf("me = %+v", me)
	}
	rec = do(t, h, "POST", "/api/portfolio/transactions", `{"symbol":"voo","side":"buy","quantity":2,"price":100,"fee":1,"date":"2026-01-02"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body)
	}
	v := decode[PortfolioView](t, rec)
	if len(v.Holdings) != 1 || v.Holdings[0].Name != "VOO" || !v.Holdings[0].Live || v.Totals.CostBasis != 201 {
		t.Errorf("portfolio = %+v", v)
	}
	if rec := do(t, h, "POST", "/api/portfolio/transactions", `{"symbol":"VOO","side":"sell","quantity":5,"price":100,"date":"2026-01-03"}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("oversell status = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/portfolio/transactions/"+v.Transactions[0].ID, "", cookie); rec.Code != http.StatusOK {
		t.Errorf("delete status = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/portfolio/transactions/nope", "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing status = %d", rec.Code)
	}
}

// 从白名单移除后，已签发的会话立即失效
func TestAllowlistRevokesSession(t *testing.T) {
	cfg := testConfig(t)
	cfg.AllowedEmails = []string{"alice@gmail.com"}
	h := NewRouter(cfg)
	rec := httptest.NewRecorder()
	auth.NewSessions(cfg.SessionSecret).Issue(rec, httptest.NewRequest("GET", "/", nil), auth.User{Sub: "2", Email: "bob@gmail.com"})
	cookie := rec.Result().Cookies()[0]
	if rec := do(t, h, "GET", "/api/portfolio", "", cookie); rec.Code != http.StatusForbidden {
		t.Errorf("portfolio status = %d, want 403", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/me", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("me status = %d, want 401", rec.Code)
	}
}

func TestWriteJSONEncodeError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, map[string]float64{"x": math.NaN()})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestStaticFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644)
	router := NewRouter(Config{StaticDir: dir})

	for path, want := range map[string]string{"/": "app", "/review": "app", "/app.js": "console.log"} {
		if rec := do(t, router, "GET", path, ""); !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: body = %q, want %q", path, rec.Body.String(), want)
		}
	}
	if rec := do(t, router, "GET", "/api/health", ""); !strings.Contains(rec.Body.String(), "ok") {
		t.Errorf("/api/health shadowed by static handler: %q", rec.Body.String())
	}
}
