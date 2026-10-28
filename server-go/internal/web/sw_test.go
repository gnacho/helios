package web

import (
	"os"
	"strings"
	"testing"
)

func TestServiceWorkerDoesNotCacheNavigations(t *testing.T) {
	src, err := os.ReadFile("assets/root/sw.js")
	if err != nil {
		t.Fatalf("read service worker: %v", err)
	}
	text := string(src)
	if strings.Contains(text, "helios-shell-v1") {
		t.Fatal("el service worker conserva la caché de navegación v1")
	}
	pages := strings.Index(text, "// Pages:")
	assets := strings.Index(text, "// Assets:")
	if pages < 0 || assets <= pages {
		t.Fatal("no se pudo aislar el bloque de navegación del service worker")
	}
	if block := text[pages:assets]; strings.Contains(block, "cache.put") {
		t.Fatal("el bloque de navegación guarda HTML en caché")
	}
}
