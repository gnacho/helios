package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// El recálculo es un subcomando explícito: sin token de HA no hay balance con el
// que recalcular y debe fallar con un motivo claro en vez de arrancar el server.
func TestRunRecomputeChargerPVNeedsHA(t *testing.T) {
	t.Setenv("HELIOS_DB_PATH", filepath.Join(t.TempDir(), "helios.db"))
	t.Setenv("HAOS_TOKEN", "")

	var out bytes.Buffer
	err := runRecomputeChargerPV(nil, &out)
	if err == nil || !strings.Contains(err.Error(), "HAOS_TOKEN") {
		t.Fatalf("err = %v (debe exigir HAOS_TOKEN)", err)
	}
	if out.Len() != 0 {
		t.Fatalf("salida = %q (no debe imprimir resumen si falla)", out.String())
	}
}

// Un flag desconocido no debe ejecutar nada.
func TestRunRecomputeChargerPVRejectsUnknownFlag(t *testing.T) {
	t.Setenv("HELIOS_DB_PATH", filepath.Join(t.TempDir(), "helios.db"))

	if err := runRecomputeChargerPV([]string{"-no-existe"}, &bytes.Buffer{}); err == nil {
		t.Fatal("un flag desconocido debe fallar")
	}
}
