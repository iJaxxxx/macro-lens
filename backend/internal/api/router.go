package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// NewRouter 返回整个站点的路由。staticDir 非空时，同时托管前端打包产物（frontend/dist）。
func NewRouter(staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, assets)
	})
	if staticDir != "" {
		// "/" 匹配所有没被上面 /api 路由接住的请求
		mux.Handle("GET /", spaHandler(staticDir))
	}
	return mux
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
