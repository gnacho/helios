package extensions

import "testing"

func TestStoreOnChangeFires(t *testing.T) {
	s := NewStore()

	var fired int
	var gotEnabled bool
	s.OnChange = func() {
		fired++
		gotEnabled = s.Get().Enabled
	}

	cfg := GenericExtensions
	cfg.Enabled = true
	s.Set(cfg)

	if fired != 1 {
		t.Fatalf("OnChange disparado %d veces, want 1", fired)
	}
	if !gotEnabled {
		t.Fatal("OnChange debe ver la config recién guardada")
	}

	// Hook nil no debe panicar.
	s.OnChange = nil
	s.Set(cfg)
}

func TestStoreOnChangeFiresPerSet(t *testing.T) {
	s := NewStore()
	var fired int
	s.OnChange = func() { fired++ }

	s.Set(GenericExtensions)
	s.Set(GenericExtensions)

	if fired != 2 {
		t.Fatalf("OnChange disparado %d veces, want 2 (uno por Set)", fired)
	}
}
