package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/iJaxxxx/macro-lens/backend/internal/auth"
	"github.com/iJaxxxx/macro-lens/backend/internal/portfolio"
)

type accountHandlers struct {
	verifier *auth.Verifier // nil 表示没配置 GOOGLE_CLIENT_ID，登录功能关闭
	sessions *auth.Sessions
	allowed  []string // 允许登录的邮箱白名单，为空表示不限制
	store    *portfolio.Store
	market   *marketCache
}

func (h *accountHandlers) login(w http.ResponseWriter, r *http.Request) {
	if h.verifier == nil {
		writeError(w, http.StatusServiceUnavailable, "服务器未配置 Google 登录（GOOGLE_CLIENT_ID）")
		return
	}
	var body struct {
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil || body.Credential == "" {
		writeError(w, http.StatusBadRequest, "缺少 Google 登录凭证")
		return
	}
	u, err := h.verifier.Verify(r.Context(), body.Credential)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Google 登录校验失败："+err.Error())
		return
	}
	if !h.isAllowed(u.Email) {
		writeError(w, http.StatusForbidden, u.Email+" 不在允许登录的名单里")
		return
	}
	h.sessions.Issue(w, r, u)
	writeJSON(w, u)
}

func (h *accountHandlers) isAllowed(email string) bool {
	return len(h.allowed) == 0 || slices.Contains(h.allowed, strings.ToLower(email))
}

func (h *accountHandlers) logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.Clear(w, r)
	writeJSON(w, map[string]bool{"ok": true})
}

func (h *accountHandlers) me(w http.ResponseWriter, r *http.Request) {
	u, err := h.sessions.User(r)
	if err != nil || !h.isAllowed(u.Email) {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, u)
}

// requireUser 包一层登录校验，未登录返回 401。
func (h *accountHandlers) requireUser(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := h.sessions.User(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if !h.isAllowed(u.Email) {
			h.sessions.Clear(w, r)
			writeError(w, http.StatusForbidden, u.Email+" 不在允许登录的名单里")
			return
		}
		next(w, r, u)
	}
}

type HoldingView struct {
	portfolio.Holding
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	PrevClose     float64 `json:"prevClose"`
	MarketValue   float64 `json:"marketValue"`
	Unrealized    float64 `json:"unrealized"`
	UnrealizedPct float64 `json:"unrealizedPct"`
	DayChange     float64 `json:"dayChange"`
	Live          bool    `json:"live"` // false 表示拉不到报价，按成本价估值
}

type PortfolioView struct {
	Transactions []portfolio.Transaction `json:"transactions"`
	Holdings     []HoldingView           `json:"holdings"`
	Totals       struct {
		MarketValue float64 `json:"marketValue"`
		CostBasis   float64 `json:"costBasis"`
		Unrealized  float64 `json:"unrealized"`
		Realized    float64 `json:"realized"`
		DayChange   float64 `json:"dayChange"`
	} `json:"totals"`
}

func (h *accountHandlers) view(ctx context.Context, u auth.User) (PortfolioView, error) {
	txs, err := h.store.List(u.Sub)
	if err != nil {
		return PortfolioView{}, err
	}
	holdings, err := portfolio.Holdings(txs)
	if err != nil {
		return PortfolioView{}, err
	}
	var v PortfolioView
	v.Transactions = slices.Clone(txs)
	slices.SortStableFunc(v.Transactions, func(a, b portfolio.Transaction) int { return strings.Compare(b.Date, a.Date) })

	v.Holdings = make([]HoldingView, len(holdings))
	var wg sync.WaitGroup
	for i, hd := range holdings {
		hv := &v.Holdings[i]
		hv.Holding, hv.Price, hv.PrevClose = hd, hd.AvgCost, hd.AvgCost
		if t, ok := findTracked(hd.Symbol); ok {
			hv.Name = t.sample.Name
		}
		if hd.Quantity == 0 || h.market == nil {
			continue
		}
		wg.Go(func() {
			if q, err := h.market.quotes.get(ctx, hd.Symbol); err == nil {
				hv.Price, hv.PrevClose, hv.Live = q.Price, q.PrevClose, true
			}
		})
	}
	wg.Wait()
	for i := range v.Holdings {
		hv := &v.Holdings[i]
		hv.MarketValue = hv.Quantity * hv.Price
		hv.Unrealized = hv.MarketValue - hv.CostBasis
		if hv.CostBasis > 0 {
			hv.UnrealizedPct = hv.Unrealized / hv.CostBasis
		}
		if hv.Live && hv.PrevClose > 0 {
			hv.DayChange = hv.Quantity * (hv.Price - hv.PrevClose)
		}
		v.Totals.MarketValue += hv.MarketValue
		v.Totals.CostBasis += hv.CostBasis
		v.Totals.Unrealized += hv.Unrealized
		v.Totals.Realized += hv.Realized
		v.Totals.DayChange += hv.DayChange
	}
	return v, nil
}

func (h *accountHandlers) getPortfolio(w http.ResponseWriter, r *http.Request, u auth.User) {
	v, err := h.view(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取持仓失败")
		return
	}
	writeJSON(w, v)
}

func (h *accountHandlers) addTransaction(w http.ResponseWriter, r *http.Request, u auth.User) {
	var t portfolio.Transaction
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if _, err := h.store.Add(u.Sub, t); err != nil {
		h.storeError(w, err)
		return
	}
	h.getPortfolio(w, r, u)
}

func (h *accountHandlers) deleteTransaction(w http.ResponseWriter, r *http.Request, u auth.User) {
	if err := h.store.Delete(u.Sub, r.PathValue("id")); err != nil {
		h.storeError(w, err)
		return
	}
	h.getPortfolio(w, r, u)
}

func (h *accountHandlers) storeError(w http.ResponseWriter, err error) {
	var ve portfolio.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "记录不存在")
	default:
		writeError(w, http.StatusInternalServerError, "保存失败")
	}
}
