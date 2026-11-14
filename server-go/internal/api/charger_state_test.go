package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"helios-server/internal/extensions"
	"helios-server/internal/ha"
)

type fakeExtHA struct {
	states map[string]string
}

func (f *fakeExtHA) GetState(id string) (ha.EntityState, bool) {
	v, ok := f.states[id]
	if !ok {
		return ha.EntityState{}, false
	}
	st := ha.EntityState{State: v, LastUpdated: time.Now().UTC().Format(time.RFC3339)}
	return st, true
}

func (f *fakeExtHA) IsConnected() bool { return true }

func (f *fakeExtHA) StatisticsDuringPeriod(string, string, []string, string, []string) (ha.Stats, error) {
	return ha.Stats{}, nil
}

func (f *fakeExtHA) HistoryDuringPeriod(string, string, string) ([]ha.HistoryRow, error) {
	return nil, nil
}

func (f *fakeExtHA) CallService(string, string, map[string]any) error { return nil }

func chargerConfig(enabled bool) extensions.Config {
	cfg := extensions.Config{Enabled: enabled}
	cfg.CarCharger.Enabled = enabled
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key:            "main",
		Name:           "Coche",
		PowerID:        "sensor.potencia",
		PowerUnit:      extensions.UnitKW,
		StateID:        "sensor.estado",
		ChargingStates: []string{"charging"},
	}}
	return cfg
}

func decodeChargerState(t *testing.T, body []byte) (bool, []extensions.LiveCharger) {
	t.Helper()
	var payload struct {
		Active   bool                     `json:"active"`
		Chargers []extensions.LiveCharger `json:"chargers"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return payload.Active, payload.Chargers
}

func TestChargerStateReportsLiveValues(t *testing.T) {
	store := extensions.NewStore()
	store.Set(chargerConfig(true))
	s := &Server{
		ExtStore: store,
		ExtHA:    &fakeExtHA{states: map[string]string{"sensor.potencia": "2.6", "sensor.estado": "charging"}},
	}

	rec := httptest.NewRecorder()
	s.handleChargerState(rec, httptest.NewRequest(http.MethodGet, "/api/charger/state", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	active, chargers := decodeChargerState(t, rec.Body.Bytes())
	if !active {
		t.Fatal("active = false with a configured charger")
	}
	if len(chargers) != 1 {
		t.Fatalf("chargers = %d, want 1", len(chargers))
	}
	if !chargers[0].Charging || !chargers[0].Connected {
		t.Fatalf("charging/connected = %v/%v", chargers[0].Charging, chargers[0].Connected)
	}
	if chargers[0].PowerKw != 2.6 {
		t.Fatalf("powerKw = %v", chargers[0].PowerKw)
	}
	if chargers[0].Name != "Coche" {
		t.Fatalf("name = %q", chargers[0].Name)
	}
}

func TestChargerStateWithExtensionOff(t *testing.T) {
	store := extensions.NewStore()
	store.Set(chargerConfig(false))
	s := &Server{ExtStore: store, ExtHA: &fakeExtHA{states: map[string]string{}}}

	rec := httptest.NewRecorder()
	s.handleChargerState(rec, httptest.NewRequest(http.MethodGet, "/api/charger/state", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an empty list is not an error)", rec.Code)
	}
	active, chargers := decodeChargerState(t, rec.Body.Bytes())
	if active {
		t.Fatal("active = true with the extension off")
	}
	if len(chargers) != 0 {
		t.Fatalf("chargers = %d, want 0", len(chargers))
	}
	if got := rec.Body.String(); got == "" || got[0] != '{' {
		t.Fatalf("unexpected body: %s", got)
	}
}
