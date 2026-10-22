package main

import (
	"log"
	"net/http"

	"helios-server/internal/config"
	"helios-server/internal/db"
	"helios-server/internal/health"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	mux.Handle("/health", &health.Handler{DB: database})

	addr := ":8080"
	log.Printf("helios-server go listening on %s (db=%s)", addr, cfg.DBPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
