package config

import "testing"

func TestLoadReadsHAInsecureOptIn(t *testing.T) {
	t.Setenv("HELIOS_DATA_DIR", t.TempDir())
	t.Setenv("HAOS_URL", "http://192.168.10.244:8123")

	if cfg, err := Load(); err != nil {
		t.Fatalf("load: %v", err)
	} else if cfg.HAOSAllowInsecure {
		t.Fatal("la excepción de transporte debe estar desactivada por defecto")
	}

	t.Setenv("HAOS_ALLOW_INSECURE", "true")
	if cfg, err := Load(); err != nil {
		t.Fatalf("load: %v", err)
	} else if !cfg.HAOSAllowInsecure {
		t.Fatal("HAOS_ALLOW_INSECURE=true debería activar la excepción")
	}

	t.Setenv("HAOS_ALLOW_INSECURE", "0")
	if cfg, err := Load(); err != nil {
		t.Fatalf("load: %v", err)
	} else if cfg.HAOSAllowInsecure {
		t.Fatal("HAOS_ALLOW_INSECURE=0 debería desactivar la excepción")
	}
}
