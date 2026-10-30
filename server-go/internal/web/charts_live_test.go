package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helios-server/internal/ha"
)

// Las graficas se pintan en el servidor: sin data-on-interval se quedan
// congeladas hasta recargar, que es el fallo que arreglan estos endpoints.
func TestPagesPollTheirCharts(t *testing.T) {
	_, r := chargerTestUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	cases := []struct{ path, want string }{
		{"/", "/ui/day-chart"},
		{"/cargador", "/ui/chg-curve?key=main"},
		{"/bateria", "/ui/bat-charts"},
		{"/inversores?inv=compare", "/ui/inv-charts?key=compare"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: status = %d", c.path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "data-on-interval__duration.60s") || !strings.Contains(body, c.want) {
			t.Fatalf("%s: falta el polling %q", c.path, c.want)
		}
	}
}

// Cada endpoint responde con un parche SSE dirigido al contenedor de su
// grafica; el navegador reemplaza ese nodo sin recargar.
func TestLiveChartFragments(t *testing.T) {
	u, r := chargerTestUI(t)
	u.ExtHA = stubChargerHA{states: map[string]ha.EntityState{
		"sensor.p": {State: "1500", LastUpdated: "2026-10-30T10:00:00Z"},
	}}
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	cases := []struct{ path, sel string }{
		{"/ui/chg-curve?key=main", "chg-curve-main"},
		{"/ui/day-chart", "day-chart"},
		{"/ui/bat-charts", "bat-power"},
		{"/ui/bat-charts", "soc-day"},
		{"/ui/inv-charts?key=compare", "inv-compare"},
		{"/ui/inv-charts?key=solis", "inv-day-solis"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", c.path, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "selector #"+c.sel) {
			t.Fatalf("%s: el parche no apunta a %s", c.path, c.sel)
		}
		if !strings.Contains(body, "<svg") {
			t.Fatalf("%s: el parche no lleva la grafica dibujada", c.path)
		}
	}

	// Un cargador que no existe no dibuja nada: 404 en vez de una curva vacia.
	req := httptest.NewRequest("GET", "/ui/inv-charts?key=nope", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("inversor inexistente: status = %d (esperado 404)", rec.Code)
	}
}
