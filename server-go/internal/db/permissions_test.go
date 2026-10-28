package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRestrictsDataDirectoryAndDatabasePermissions(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "data")
	path := filepath.Join(dir, "helios.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("data dir mode = %#o, esperado %#o", perm, 0o700)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("database mode = %#o, esperado %#o", perm, 0o600)
	}
}

func TestOpenTightensExistingWorldReadableFiles(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "helios.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("seed db file: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("existing data dir mode = %#o, esperado %#o", perm, 0o700)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("existing database mode = %#o, esperado %#o", perm, 0o600)
	}
}
