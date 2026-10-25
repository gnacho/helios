package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

func httpNoop() *noopHandler { return &noopHandler{} }

type noopHandler struct{}

func (n *noopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/api.db")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestExtensionsPayloadIncludesByd(t *testing.T) {
	d := testDB(t)
	store := extensions.NewStore()
	store.Set(extensions.ResolveConfig(d))
	sol := solar.New(nil, nil)
	srv := NewServer(&auth.Auth{}, d, t.TempDir(), sol, nil, extensions.New(store, d, nil), store, nil, solar.PriceConfig{}, nil)
	srv.SSE = httpNoop()

	req := httptest.NewRequest("GET", "/api/extensions", nil)
	rec := httptest.NewRecorder()
	// handler directo: el middleware de auth se valida aparte
	srv.handleGetExtensions(rec, req)

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := payload["byd"]; !ok {
		t.Fatalf("falta byd en el payload: %v", payload)
	}
	if _, ok := payload["bydActive"]; !ok {
		t.Fatalf("falta bydActive")
	}
	byd, _ := payload["byd"].(map[string]any)
	if _, ok := byd["enabled"]; !ok {
		t.Fatalf("byd sin enabled: %v", byd)
	}
}
