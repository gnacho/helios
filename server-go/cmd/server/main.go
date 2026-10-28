package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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
	"helios-server/internal/web"
)

type livePayload struct {
	solar.LiveData
	Chargers []extensions.LiveCharger `json:"chargers"`
	Byd      *extensions.BydLive      `json:"byd"`
}

// newHAClient builds the Home Assistant client after enforcing transport rules.
func newHAClient(cfg *config.Config) (*ha.Client, error) {
	if cfg.HAOSToken == "" {
		return nil, nil
	}
	if err := ha.ValidateHAURL(cfg.HAOSURL, cfg.HAOSAllowInsecure); err != nil {
		return nil, err
	}
	return ha.NewClient(cfg.HAOSURL, cfg.HAOSToken), nil
}

func main() {
	// Recálculo único del reparto FV del cargador: subcomando explícito, nunca
	// parte del arranque normal.
	if len(os.Args) > 1 && os.Args[1] == cmdRecomputeChargerPV {
		if err := runRecomputeChargerPV(os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("recompute-charger-pv: %v", err)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	trustedProxies, err := auth.ParseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		log.Fatalf("trusted proxies: %v", err)
	}
	a := &auth.Auth{
		DB:             database,
		SessionTTL:     cfg.SessionTTL,
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		TrustedProxies: trustedProxies,
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
		haClient, err := newHAClient(cfg)
		if err != nil {
			log.Fatalf("[ha] %v", err)
		}
		client = haClient

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
			log.Printf("[ha] conectado a %s", ha.LogSafeURL(cfg.HAOSURL))
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
	// Los precios editados en Ajustes viven en KV y ganan sobre el .env.
	if database != nil {
		if v, err := database.KVGet("priceImportEur"); err == nil && v != "" {
			if f, perr := strconv.ParseFloat(v, 64); perr == nil && f > 0 {
				prices.PriceImport = f
			}
		}
		if v, err := database.KVGet("priceExportEur"); err == nil && v != "" {
			if f, perr := strconv.ParseFloat(v, 64); perr == nil && f >= 0 {
				prices.PriceExport = f
			}
		}
		if v, err := database.KVGet("co2Factor"); err == nil && v != "" {
			if f, perr := strconv.ParseFloat(v, 64); perr == nil && f > 0 {
				prices.Co2PerKwh = f
			}
		}
	}

	var haReader solar.HAReader
	var extHA extensions.HA
	if client != nil {
		haReader = client
		extHA = client
	}

	srv := api.NewServer(a, database, cfg.DataDir, sol, haReader, ext, extStore, extHA, prices, store)
	srv.Push = pushSvc
	srv.SSE = hub

	// La UI es la de templ: la SPA React se retiró (ya no hay vuelta atrás).
	ui := web.New(a, database, sol, haReader, store, extStore, ext, extHA, prices, hub)
	ui.DataDir = cfg.DataDir
	ui.Push = pushSvc
	// Las fotos heredadas del React solo se escalaron: recorta el aire
	// sobrante una vez para que ocupen el alto de la tarjeta.
	ui.OptimizeStoredChargerImages()
	srv.UI = ui
	log.Printf("[helios] UI templ servida por el binario")

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("helios-server go listening on %s (db=%s)", addr, cfg.DBPath)
	httpSrv := newHTTPServer(addr, srv.Routes())
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
		// WriteTimeout stays disabled: SSE streams are long-lived.
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

	// Consolidación del histórico diario: cada 30 min (idempotente). El cierre
	// de ayer se escribe en cuanto HA publica las estadísticas del día, sin
	// depender de una hora exacta ni de que el proceso estuviera vivo a las
	// 00:00; además rellena cualquier hueco previo.
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[backfill] panic en consolidación: %v", r)
					}
				}()
				if rows, err := sol.ConsolidateDaily(client); err != nil {
					log.Printf("[backfill] consolidación: %v", err)
				} else if rows > 0 {
					log.Printf("[backfill] consolidación: %d filas", rows)
				}
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
		if rows, err := sol.ConsolidateDaily(client); err != nil {
			log.Printf("[backfill] error: %v", err)
		} else if rows > 0 {
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
