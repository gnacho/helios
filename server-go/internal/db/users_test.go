package db

import (
	"path/filepath"
	"testing"
)

// TestAuditRangeWithNullUser is the regression test for the latent
// AuditRange bug: rows inserted with an empty userID are stored as NULL
// (nullIfEmpty) and the plain Scan to string failed with "converting NULL
// to string" (the audit endpoint returned 500 with real data).
func TestAuditRangeWithNullUser(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	if err := d.Audit("nacho", "user-1", "auth.login", nil); err != nil {
		t.Fatalf("audit 1: %v", err)
	}
	if err := d.Audit("system", "", "update.apply", nil); err != nil {
		t.Fatalf("audit 2: %v", err)
	}

	rows, err := d.AuditRange(10, 0)
	if err != nil {
		t.Fatalf("AuditRange: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d (esperado 2)", len(rows))
	}
	// La más reciente primero (system, user_id NULL -> "").
	if rows[0].Actor != "system" || rows[0].UserID != "" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].Actor != "nacho" || rows[1].UserID != "user-1" {
		t.Fatalf("row1 = %+v", rows[1])
	}
}
