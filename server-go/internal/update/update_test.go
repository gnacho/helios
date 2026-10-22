package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"helios-server/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int // signo: negativo si a<b, 0 si igual, positivo si a>b
	}{
		{"0.8.28", "0.8.29", -1},
		{"0.9.0", "0.8.99", 1},
		{"1.0.0", "1.0.0", 0},
		{"v0.8.2", "0.8.10", -1}, // v prefijo + orden numérico (2 < 10)
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		sign := 0
		if got < 0 {
			sign = -1
		} else if got > 0 {
			sign = 1
		}
		if sign != c.want {
			t.Fatalf("compare(%q,%q) = %d (signo esperado %d)", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	v := parseVersion("v1.2.3")
	if v != [3]int{1, 2, 3} {
		t.Fatalf("v = %v", v)
	}
	v2 := parseVersion("abc")
	if v2 != [3]int{0, 0, 0} {
		t.Fatalf("v2 = %v", v2)
	}
}

func TestRequestAndReadProgress(t *testing.T) {
	dir := t.TempDir()
	if !RequestUpdate(dir) {
		t.Fatal("requestUpdate falló")
	}
	if _, err := os.Stat(filepath.Join(dir, ".update-requested")); err != nil {
		t.Fatalf("flag no creado: %v", err)
	}

	// Progreso fresco
	p := ProgressState{Ts: time.Now().UnixMilli(), Step: "descargando", Pct: 42}
	raw, _ := json.Marshal(p)
	os.WriteFile(filepath.Join(dir, progressFile), raw, 0o644)

	got := ReadProgress(dir)
	if got == nil || got.Step != "descargando" || got.Pct != 42 {
		t.Fatalf("progress = %+v", got)
	}

	// Progreso viejo -> null
	old := ProgressState{Ts: time.Now().Add(-20 * time.Minute).UnixMilli(), Step: "stale"}
	raw2, _ := json.Marshal(old)
	os.WriteFile(filepath.Join(dir, progressFile), raw2, 0o644)
	if got := ReadProgress(dir); got != nil {
		t.Fatalf("progreso viejo debería ser null: %+v", got)
	}
}

func TestStatusNoCurrentNoAvailable(t *testing.T) {
	d := testDB(t)
	dir := t.TempDir()

	// Sin marker y sin red (LatestRelease falla silenciosamente -> latest null)
	Marker = filepath.Join(dir, "no-existe")
	st, err := GetStatus(d, dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Available {
		t.Fatal("sin current no puede haber available")
	}
	if st.CanApply {
		t.Fatal("sin current no puede aplicar")
	}
}

func TestWatchProgressEmits(t *testing.T) {
	dir := t.TempDir()
	stop := WatchProgress(dir)
	defer stop()

	got := make(chan ProgressPayload, 4)
	unsub := Subscribe(func(p ProgressPayload) { got <- p })
	defer unsub()

	// Escribir progreso tras un momento (mtime distinto)
	time.Sleep(600 * time.Millisecond)
	p := ProgressState{Ts: time.Now().UnixMilli(), Step: "aplicando", Pct: 80}
	raw, _ := json.Marshal(p)
	os.WriteFile(filepath.Join(dir, progressFile), raw, 0o644)

	select {
	case payload := <-got:
		if payload.Type != "progress" || payload.Step != "aplicando" || payload.Pct != 80 {
			t.Fatalf("payload = %+v", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no llegó evento de progreso")
	}
}
