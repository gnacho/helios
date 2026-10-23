package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestMergeDebug(t *testing.T) {
	u, r := ajustesTestUI(t)
	// Sembrar el KV con otras claves, como la instancia real.
	if err := u.DB.KVSet("install_config", `{"solisKwp":4.4,"priceImportEur":0.15}`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cookie := doLoginAs(t, r, "nacho", "password1")
	form := url.Values{"inv.count": {"1"}, "inv.0.name": {"X"}, "battery.enabled": {"1"}}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/topology", form)
	t.Logf("status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	raw, _ := u.DB.KVGet("install_config")
	t.Logf("kv=%s", raw)
	if !strings.Contains(raw, "solisKwp") {
		t.Fatal("merge perdido")
	}
}
