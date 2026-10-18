package main

import (
	"log"
	"net/http"
	"os"

	"github.com/iJaxxxx/macro-lens/backend/internal/api"
)

func main() {
	addr := ":" + envOr("PORT", "8080")
	log.Printf("macro-lens backend listening on %s", addr)
	if err := http.ListenAndServe(addr, api.NewRouter()); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
