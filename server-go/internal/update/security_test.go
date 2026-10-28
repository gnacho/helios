package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeProgress(t *testing.T, dir string, ts int64) {
	t.Helper()
	raw, err := json.Marshal(ProgressState{Ts: ts, Step: "download", Pct: 15})
	if err != nil {
		t.Fatalf("marshal progress: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, progressFile), raw, 0o644); err != nil {
		t.Fatalf("write progress: %v", err)
	}
}

func TestReadProgressAcceptsSecondTimestamps(t *testing.T) {
	dir := t.TempDir()
	writeProgress(t, dir, time.Now().Unix())

	got := ReadProgress(dir)
	if got == nil {
		t.Fatal("un progreso fresco en segundos se descartó como viejo")
	}
	if got.Step != "download" || got.Pct != 15 {
		t.Fatalf("progress = %+v", got)
	}
}

func TestReadProgressStillAcceptsMillisecondTimestamps(t *testing.T) {
	dir := t.TempDir()
	writeProgress(t, dir, time.Now().UnixMilli())

	if got := ReadProgress(dir); got == nil {
		t.Fatal("un progreso fresco en milisegundos se descartó como viejo")
	}
}

func TestReadProgressRejectsStaleSecondsAndMilliseconds(t *testing.T) {
	dir := t.TempDir()

	writeProgress(t, dir, time.Now().Add(-20*time.Minute).Unix())
	if got := ReadProgress(dir); got != nil {
		t.Fatalf("progreso viejo en segundos debería ser nulo: %+v", got)
	}

	writeProgress(t, dir, time.Now().Add(-20*time.Minute).UnixMilli())
	if got := ReadProgress(dir); got != nil {
		t.Fatalf("progreso viejo en milisegundos debería ser nulo: %+v", got)
	}
}
