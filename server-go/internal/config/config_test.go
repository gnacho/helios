package config

import (
	"reflect"
	"testing"
)

func TestLoadParsesTrustedProxies(t *testing.T) {
	t.Setenv("HELIOS_DATA_DIR", t.TempDir())
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1, 10.0.0.0/8")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"127.0.0.1", "10.0.0.0/8"}
	if !reflect.DeepEqual(cfg.TrustedProxies, want) {
		t.Fatalf("TrustedProxies = %#v, want %#v", cfg.TrustedProxies, want)
	}
}
