package main

import (
	"log"
	"net/http"
	"os"

	"fmt"
	"helios-server/internal/api"
	"helios-server/internal/auth"
	"helios-server/internal/config"
	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
	"helios-server/internal/solar"
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
		SessionTTL:    cfg.SessionTTL,
		SessionSecret: os.Getenv("SESSION_SECRET"),
	}

	if err := ensureBootstrapAdmin(database, a); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	// Topología resuelta (kv install_config o perfil legacy/genérico).
	store := install.NewStore()
	store.Set(install.ResolveInstall(database))

	sol := solar.New(store, database)

	// Cliente HAOS (opcional: sin token la API solar responde 503).
	var haReader solar.HAReader
	if cfg.HAOSToken != "" {
		client := ha.NewClient(cfg.HAOSURL, cfg.HAOSToken)
		client.OnConnected = func() {
			log.Printf("[ha] conectado a %s", cfg.HAOSURL)
			if err := client.SubscribeEntities(store.LiveEntities()); err != nil {
				log.Printf("[ha] subscribe: %v", err)
			}
		}
		client.OnDisconnect = func() { log.Printf("[ha] desconectado") }
		client.OnFatal = func(msg string) { log.Printf("[ha] fatal: %s", msg) }
		client.OnWSError = func(msg string) { log.Printf("[ha] ws: %s", msg) }
		client.Start()
		defer client.Stop()
		haReader = client
	} else {
		log.Printf("[ha] HAOS_TOKEN no definido: API solar deshabilitada")
	}

	prices := solar.PriceConfig{
		PriceImport: cfg.PriceImport,
		PriceExport: cfg.PriceExport,
		Co2PerKwh:   cfg.Co2PerKwh,
	}

	srv := api.NewServer(a, database, cfg.DataDir, sol, haReader, prices)

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
