package api

import (
	"encoding/json"
	"net/http"
)

// Asset 是首页行情卡片的一项。
type Asset struct {
	Name   string `json:"name"`
	Code   string `json:"code"`
	Price  string `json:"price"`
	Change string `json:"change"`
	Up     bool   `json:"up"`
}

// 示例数据，与前端设计稿保持一致；后续替换为真实行情源。
var assets = []Asset{
	{Name: "标普 500", Code: "S&P 500", Price: "5,842.3", Change: "+0.62%", Up: true},
	{Name: "现货黄金", Code: "XAU / USD", Price: "2,687.4", Change: "+1.84%", Up: true},
	{Name: "WTI 原油", Code: "CL1!", Price: "68.92", Change: "-2.15%", Up: false},
}

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, assets)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
