package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ZohaibSoomro/edi-bridge/internal/handlers"
	"github.com/ZohaibSoomro/edi-bridge/internal/middleware"
)

func newHandler() http.Handler {
	// Seed a local test key so you can verify SaaS Bearer auth & quota headers immediately
	keyStore := middleware.NewMemoryKeyStore([]*middleware.KeyRecord{
		{
			Key:          "edix_live_test_zohaib_001",
			TenantID:     "tenant_demo",
			Plan:         "starter",
			MonthlyLimit: 25,
			UsedThisMo:   0,
			Active:       true,
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"edi-bridge"}`))
	})

	mux.Handle("/v1/parse", middleware.DualAuth(keyStore, http.HandlerFunc(handlers.HandleParse)))
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      newHandler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("EDI-Bridge API listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
