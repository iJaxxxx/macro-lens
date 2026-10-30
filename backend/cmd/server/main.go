package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	_ "time/tzdata" // 服务器上不一定装了时区数据，内置一份，按纽约时间划分交易日要用

	"github.com/iJaxxxx/macro-lens/backend/internal/api"
	"github.com/iJaxxxx/macro-lens/backend/internal/auth"
	"github.com/iJaxxxx/macro-lens/backend/internal/llm"
	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

func main() {
	addr := ":" + envOr("PORT", "8080")
	// STATIC_DIR 指向前端打包产物；本地开发用 Vite，不需要设置
	staticDir := os.Getenv("STATIC_DIR")
	// DATA_DIR 存放用户的交易记录和会话密钥
	dataDir := envOr("DATA_DIR", "data")
	secret, err := auth.LoadSecret(os.Getenv("SESSION_SECRET"), dataDir)
	if err != nil {
		log.Fatalf("session secret: %v", err)
	}

	yahoo := market.NewYahoo()
	// 可以指向自建的代理或本地模拟服务
	if v := os.Getenv("YAHOO_BASE_URL"); v != "" {
		yahoo.BaseURL = v
	}
	cfg := api.Config{
		StaticDir:      staticDir,
		Market:         yahoo,
		LLM:            llm.FromEnv(),
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		SessionSecret:  secret,
		DataDir:        dataDir,
	}
	if v := os.Getenv("ALLOWED_EMAILS"); v != "" {
		cfg.AllowedEmails = strings.Split(v, ",")
	}
	log.Printf("macro-lens backend listening on %s (static: %q, data: %q, llm: %v, google login: %v)",
		addr, staticDir, dataDir, cfg.LLM != nil, cfg.GoogleClientID != "")
	if err := http.ListenAndServe(addr, api.NewRouter(cfg)); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
