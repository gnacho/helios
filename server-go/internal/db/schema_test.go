package db

import (
	"path/filepath"
	"testing"
)

func TestLoginAttemptsHasLockedUntilIndex(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	var count int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_login_attempts_locked_until'`).Scan(&count)
	if err != nil {
		t.Fatalf("query index: %v", err)
	}
	if count != 1 {
		t.Fatalf("login_attempts locked_until index count = %d, want 1", count)
	}
}
