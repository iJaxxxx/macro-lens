package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// NewRouter 返回整个站点的路由。staticDir 非空时，同时托管前端打包产物（frontend/dist）。
// quotes 为 nil 时 /api/assets 只返回示例数据。
func NewRouter(staticDir string, quotes QuoteFetcher) http.Handler {
	assets := &assetService{quotes: quotes}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, assets.list(r.Context()))
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
