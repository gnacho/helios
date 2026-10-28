package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"helios-server/internal/config"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// cmdRecomputeChargerPV es el subcomando de una sola pasada que reescribe el
// reparto FV del histórico del cargador con el criterio del balance. No se
// ejecuta en el arranque ni en el cron: se lanza a mano.
const cmdRecomputeChargerPV = "recompute-charger-pv"

// haConnectTimeout es lo que espera el subcomando a que HA abra el websocket.
const haConnectTimeout = 30 * time.Second

// runRecomputeChargerPV ejecuta el recálculo y escribe el resumen en out.
func runRecomputeChargerPV(args []string, out io.Writer) error {
	fs := flag.NewFlagSet(cmdRecomputeChargerPV, flag.ContinueOnError)
	days := fs.Int("days", 0, "días hacia atrás a recalcular (0 = ventana del backfill, 9 días)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.HAOSToken == "" {
		return errors.New("HAOS_TOKEN no configurado: el recálculo necesita el balance de Home Assistant")
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer database.Close()

	client, err := newHAClient(cfg)
	if err != nil {
		return fmt.Errorf("[ha] %w", err)
	}
	client.Start()
	defer client.Stop()
	if err := waitHAConnected(client, haConnectTimeout); err != nil {
		return err
	}

	store := install.NewStore()
	store.Set(install.ResolveInstall(database))
	extStore := extensions.NewStore()
	extStore.Set(extensions.ResolveConfig(database))
	ext := extensions.New(extStore, database, store)

	report, err := ext.RecomputeChargerPVShare(client, *days)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, report.String())
	return err
}

// waitHAConnected espera a que el websocket de HA acepte llamadas.
func waitHAConnected(client *ha.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if client.IsConnected() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("[ha] sin conexión en %s", timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
