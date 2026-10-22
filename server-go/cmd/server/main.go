package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"helios-server/internal/api"
	"helios-server/internal/auth"
	"helios-server/internal/config"
	"helios-server/internal/db"
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

	a := &auth.Auth{
		DB:            database,
		SessionTTL:    7 * 24 * time.Hour,
		SessionSecret: os.Getenv("SESSION_SECRET"),
	}

	// bootstrap admin
	if err := ensureBootstrapAdmin(database, a); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	srv := api.NewServer(a, database, cfg.DataDir)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("helios-server go listening on %s (db=%s)", addr, cfg.DBPath)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}

func ensureBootstrapAdmin(database *db.DB, a *auth.Auth) error {
	username := os.Getenv("AUTH_USER")
	password := os.Getenv("AUTH_PASS")
	if username == "" || password == "" {
		return nil
	}
	existing, err := database.GetUserByUsername(username)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	if _, err := a.RegisterUser(username, password, "es", "admin"); err != nil {
		return err
	}
	log.Printf("[helios] admin bootstrap creado: %s", username)
	return nil
}
