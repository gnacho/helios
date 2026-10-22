package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"helios-server/internal/alerts"
	"helios-server/internal/api"
	"helios-server/internal/auth"
	"helios-server/internal/config"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/ha"
	"helios-server/internal/install"
	"helios-server/internal/push"
	"helios-server/internal/solar"
	"helios-server/internal/sse"
)

type livePayload struct {
	solar.LiveData
	Chargers []extensions.LiveCharger `json:"chargers"`
	Byd      *extensions.BydLive      `json:"byd"`
}

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

	store := install.NewStore()
	store.Set(install.ResolveInstall(database))
	sol := solar.New(store, database)

	extStore := extensions.NewStore()
	extStore.Set(extensions.ResolveConfig(database))
	ext := extensions.New(extStore, database, store)

	pushSvc := push.New(database, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject)

	// Cliente HAOS + hub SSE + motor de alertas.
	var client *ha.Client
	var hub *sse.Hub

	if cfg.HAOSToken != "" {
		client = ha.NewClient(cfg.HAOSURL, cfg.HAOSToken)

		hub = sse.NewHub(func() []byte {
			payload := livePayload{
				LiveData: sol.ComputeLive(client),
				Chargers: extensions.ComputeChargerLive(client, extStore.Get()),
				Byd:      extensions.ComputeBydLive(client, extStore.Get()),
			}
			raw, err := json.Marshal(map[string]any{"type": "live", "data": payload})
			if err != nil {
				return []byte(`{"type":"live","data":{}}`)
			}
			return raw
		})

		client.OnConnected = func() {
			log.Printf("[ha] conectado a %s", cfg.HAOSURL)
			ids := append(store.LiveEntities(), extensions.ChargerEntities(extStore.Get())...)
			ids = append(ids, extensions.BydEntities(extStore.Get())...)
			if err := client.SubscribeEntities(ids); err != nil {
				log.Printf("[ha] subscribe: %v", err)
			}
		}
		client.OnDisconnect = func() { log.Printf("[ha] desconectado") }
		client.OnFatal = func(msg string) { log.Printf("[ha] fatal: %s", msg) }
		client.OnWSError = func(msg string) { log.Printf("[ha] ws: %s", msg) }
		client.OnEntity = func(string) { hub.Broadcast() }
		client.Start()
		defer client.Stop()

		hub.LogStart()
		startBackgroundLoops(database, client, store, ext, extStore, sol, pushSvc)
	} else {
		hub = sse.NewHub(func() []byte { return []byte(`{"type":"live","data":{}}`) })
		log.Printf("[ha] HAOS_TOKEN no definido: API solar deshabilitada")
	}

	prices := solar.PriceConfig{
		PriceImport: cfg.PriceImport,
		PriceExport: cfg.PriceExport,
		Co2PerKwh:   cfg.Co2PerKwh,
	}

	var haReader solar.HAReader
	var extHA extensions.HA
	if client != nil {
		haReader = client
		extHA = client
	}

	staticDir := getenv("STATIC_DIR", cfg.DataDir+"/public")
	srv := api.NewServer(a, database, cfg.DataDir, sol, haReader, ext, extStore, extHA, prices, store, staticDir)
	srv.Push = pushSvc
	srv.SSE = hub

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("helios-server go listening on %s (db=%s)", addr, cfg.DBPath)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}

// startBackgroundLoops arranca los ticks del motor de alertas, la acumulación
// de cargadores, el flush de la cola push y el resumen diario.
func startBackgroundLoops(database *db.DB, client *ha.Client, store *install.Store, ext *extensions.Ext, extStore *extensions.Store, sol *solar.Solar, pushSvc *push.Service) {
	engine := alerts.New(database, client, store, func(tipo string, datos push.Datos, opts push.Options) {
		pushSvc.NotifyAll(tipo, datos, opts)
	})
	engine.LiveFn = func(h alerts.HA) alerts.LiveSnapshot {
		live := sol.ComputeLive(client)
		return alerts.LiveSnapshot{
			Soc:            live.Soc,
			BatteryPower:   live.BatteryPower,
			RespaldoKw:     live.RespaldoKw,
			NoRespaldadaKw: live.NoRespaldadaKw,
		}
	}

	kpisFn := func() (alerts.KpisResumen, error) {
		k, err := sol.GetKpis(client, solar.TodayStr(), solar.PriceConfig{})
		if err != nil {
			return alerts.KpisResumen{}, err
		}
		return alerts.KpisResumen{
			ProductionKwh:  k.ProductionKwh,
			ConsumptionKwh: k.ConsumptionKwh,
			AutoconsumoPct: k.AutoconsumoPct,
		}, nil
	}

	// Tick de alertas: cada minuto.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[alerts] panic en tick: %v", r)
					}
				}()
				engine.Tick()
			}()
		}
	}()

	// Acumulación de cargadores: cada 60 s.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[extensions] panic en acumulación: %v", r)
					}
				}()
				ext.AccumulateChargerDaily(client)
			}()
		}
	}()

	// Flush de la cola push (quiet hours): cada hora.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[push] panic en flush: %v", r)
					}
				}()
				pushSvc.FlushNotificationQueue()
			}()
		}
	}()

	// Backfill inicial (solo si daily está vacío) y curvas.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[backfill] panic: %v", r)
			}
		}()
		time.Sleep(5 * time.Second)
		if ran, rows, err := sol.MaybeBackfill(client); err != nil {
			log.Printf("[backfill] error: %v", err)
		} else if ran {
			log.Printf("[backfill] histórico: %d filas", rows)
		}
		if ran, rows := sol.BackfillDaySeries(client, ha.Stats{}); ran {
			log.Printf("[backfill] series: %d días", rows)
		}
		if n := ext.BackfillChargerHistory(client); n > 0 {
			log.Printf("[backfill] cargadores: %d días", n)
		}
		_ = extStore
	}()

	engine.ScheduleResumenDiario(kpisFn)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
