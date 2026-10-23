package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"helios-server/internal/auth"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

// TestSystemInfoShape is the contract test of GET /api/system/info: the React
// Ajustes page (and now the templ page) reads node.version and app.version;
// third parties rely on the full shape. Changing it is a breaking change.
func TestSystemInfoShape(t *testing.T) {
	d := testDB(t)
	store := extensions.NewStore()
	store.Set(extensions.ResolveConfig(d))
	sol := solar.New(nil, nil)
	srv := NewServer(&auth.Auth{}, d, t.TempDir(), sol, nil, extensions.New(store, d, nil), store, nil, solar.PriceConfig{}, nil, "")
	srv.SSE = httpNoop()

	req := httptest.NewRequest("GET", "/api/system/info", nil)
	rec := httptest.NewRecorder()
	srv.handleSystemInfo(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	for _, key := range []string{"app", "node", "react", "uptime", "memory"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("falta %q en /api/system/info: %v", key, payload)
		}
	}
	app, _ := payload["app"].(map[string]any)
	if _, ok := app["version"].(string); !ok {
		t.Fatalf("app.version debe ser string: %v", app)
	}
	if _, ok := app["name"].(string); !ok {
		t.Fatalf("app.name debe ser string: %v", app)
	}
	node, _ := payload["node"].(map[string]any)
	for _, key := range []string{"version", "platform", "arch"} {
		if _, ok := node[key].(string); !ok {
			t.Fatalf("node.%s debe ser string: %v", key, node)
		}
	}
	if v, _ := node["version"].(string); v == "" {
		t.Fatalf("node.version vacío")
	}
	mem, _ := payload["memory"].(map[string]any)
	if _, ok := mem["rss"]; !ok {
		t.Fatalf("memory.rss falta: %v", mem)
	}
	if _, ok := mem["heapUsed"]; !ok {
		t.Fatalf("memory.heapUsed falta: %v", mem)
	}
	if _, ok := payload["uptime"].(float64); !ok {
		t.Fatalf("uptime debe ser numérico: %v", payload["uptime"])
	}
	if _, ok := payload["react"].(string); !ok {
		t.Fatalf("react debe ser string: %v", payload["react"])
	}
}
