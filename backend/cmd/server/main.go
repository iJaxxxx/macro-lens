package main

import (
	"log"
	"net/http"
	"os"

	"github.com/iJaxxxx/macro-lens/backend/internal/api"
	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

func main() {
	addr := ":" + envOr("PORT", "8080")
	// STATIC_DIR 指向前端打包产物；本地开发用 Vite，不需要设置
	staticDir := os.Getenv("STATIC_DIR")
	log.Printf("macro-lens backend listening on %s (static: %q)", addr, staticDir)
	if err := http.ListenAndServe(addr, api.NewRouter(staticDir, market.NewYahoo())); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
