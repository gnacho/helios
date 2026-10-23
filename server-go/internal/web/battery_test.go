package web

import (
	"math"
	"strings"
	"testing"

	"helios-server/internal/solar"
)

func TestSocGaugeGeometry(t *testing.T) {
	pal := newPalette(false)
	g := buildSocGauge("es", 84.6, pal)
	// Track: arco completo 240 grados desde 150.
	if !strings.HasPrefix(g.TrackPath, "M ") || !strings.Contains(g.TrackPath, "A 100.00 100.00 0 1 1") {
		t.Fatalf("track = %q", g.TrackPath)
	}
	// Valor: arco parcial (240 * 0.846 = 203 < 240), large=1 porque > 180.
	if g.ValuePath == "" || !strings.Contains(g.ValuePath, " 1 1 ") {
		t.Fatalf("value = %q", g.ValuePath)
	}
	// Color por umbral: 84.6 verde batería.
	if g.Color != pal.Bateria {
		t.Fatalf("color = %q", g.Color)
	}
	// Textos.
	if g.SocText != "85" {
		t.Fatalf("socText = %q", g.SocText)
	}
	if !strings.Contains(g.KwhText, "4,2") || !strings.Contains(g.KwhText, "5,0") {
		t.Fatalf("kwhText = %q", g.KwhText)
	}
	if !strings.Contains(g.Aria, "85") {
		t.Fatalf("aria = %q", g.Aria)
	}
	// Marca de reserva al 20 % del arco: ángulo 198.
	rx, ry := donutPolar(gaugeCX, gaugeCY, gaugeR-13, 198)
	if math.Abs(g.TickX1-rx) > 0.01 || math.Abs(g.TickY1-ry) > 0.01 {
		t.Fatalf("tick = (%v,%v) esperado (%v,%v)", g.TickX1, g.TickY1, rx, ry)
	}
	// SOC 0: sin arco de valor.
	g0 := buildSocGauge("es", 0, pal)
	if g0.ValuePath != "" || g0.SocText != "0" {
		t.Fatalf("gauge vacío = %q / %q", g0.ValuePath, g0.SocText)
	}
	// SOC 10: arco corto (span 24 < 180 → large=0) y rojo.
	g10 := buildSocGauge("es", 10, pal)
	if !strings.Contains(g10.ValuePath, " 0 1 ") || g10.Color != pal.RedCompra {
		t.Fatalf("gauge bajo = %q / %q", g10.ValuePath, g10.Color)
	}
}

func socTestPoints() []solar.DayPoint {
	return []solar.DayPoint{
		{T: 0, Label: "00:00", Soc: 40, BatteryPower: 0},
		{T: 5, Label: "00:05", Soc: 45, BatteryPower: 2},    // inicio de carga
		{T: 10, Label: "00:10", Soc: 99.7, BatteryPower: 2}, // llena
		{T: 15, Label: "00:15", Soc: 80, BatteryPower: -1},  // inicio descarga
		{T: 20, Label: "00:20", Soc: 70, BatteryPower: -1},
		{T: 25, Label: "00:25", Soc: 60, BatteryPower: 0},
	}
}

func TestBuildSocDayChart(t *testing.T) {
	pal := newPalette(false)
	m := buildSocDayChart("es", socTestPoints(), 12, pal)
	// Dominio fijo 0..100: 6 ticks Y.
	if len(m.TicksY) != 6 || m.TicksY[1].Label != "20" {
		t.Fatalf("ticksY = %+v", m.TicksY)
	}
	// Zona de reserva hasta el 20 %.
	wantReserve := geomBatSoc.y(20, 100)
	if math.Abs(m.ReserveY-wantReserve) > 0.01 {
		t.Fatalf("reserveY = %v", m.ReserveY)
	}
	// Pasado cierra en la baseline; futuro es línea.
	if !strings.HasSuffix(m.PastPath, "Z") || strings.HasSuffix(m.FuturePath, "Z") {
		t.Fatal("past/future mal formados")
	}
	// Tres eventos en orden: carga, llena, descarga.
	if len(m.Events) != 3 {
		t.Fatalf("events = %d: %+v", len(m.Events), m.Events)
	}
	if !strings.Contains(m.Events[0].Label, "00:05") || !strings.Contains(m.Events[1].Label, "00:10") || !strings.Contains(m.Events[2].Label, "00:15") {
		t.Fatalf("labels de eventos: %q / %q / %q", m.Events[0].Label, m.Events[1].Label, m.Events[2].Label)
	}
	// nowMin=12: los dos primeros eventos pasados, el tercero futuro.
	if !m.Events[0].Past || !m.Events[1].Past || m.Events[2].Past {
		t.Fatalf("past flags: %+v", m.Events)
	}
}

func TestBuildBatPowerChart(t *testing.T) {
	pal := newPalette(false)
	// 15 min = 3 buckets de 5 min. Carga +1.5 constante el primer tramo,
	// descarga -1 el segundo, 0 el tercero.
	points := []solar.DayPoint{
		{T: 0, BatteryPower: 1}, {T: 5, BatteryPower: 2}, {T: 10, BatteryPower: 1.5}, // media 1.5
		{T: 15, BatteryPower: -1}, {T: 20, BatteryPower: -1}, // media -1
		{T: 30, BatteryPower: 0.004}, // bajo el umbral de 0.005
	}
	m := buildBatPowerChart("es", points, pal)
	if len(m.Buckets) != 96 {
		t.Fatalf("buckets = %d", len(m.Buckets))
	}
	// Primer bucket: carga de 1.5 kW.
	b0 := m.Buckets[0]
	if b0.ChargeH <= 0 || b0.DischargeH != 0 {
		t.Fatalf("bucket 0 = %+v", b0)
	}
	wantH := batY(0) - batY(1.5)
	if math.Abs(b0.ChargeH-wantH) > 0.01 || math.Abs(b0.ChargeY-batY(1.5)) > 0.01 {
		t.Fatalf("carga h/y = %v/%v", b0.ChargeH, b0.ChargeY)
	}
	// Segundo bucket: descarga de 1 kW.
	b1 := m.Buckets[1]
	if b1.DischargeH <= 0 || b1.ChargeH != 0 {
		t.Fatalf("bucket 1 = %+v", b1)
	}
	if math.Abs(b1.DischargeH-(batY(-1)-batY(0))) > 0.01 {
		t.Fatalf("descarga h = %v", b1.DischargeH)
	}
	// Bucket con media por debajo del umbral: sin barras.
	b2 := m.Buckets[2]
	if b2.ChargeH != 0 || b2.DischargeH != 0 {
		t.Fatalf("bucket 2 = %+v", b2)
	}
	// Hueco (bucket 3, t=45..60 sin puntos): vacío.
	if m.Buckets[3].ChargeH != 0 || m.Buckets[3].DischargeH != 0 {
		t.Fatalf("bucket vacío = %+v", m.Buckets[3])
	}
	if len(m.TicksX) != 7 || len(m.TicksY) != 5 || m.TicksY[0].Label != "-2" {
		t.Fatalf("ticks: %d/%d", len(m.TicksX), len(m.TicksY))
	}
}

func TestBuildWeekBalance(t *testing.T) {
	pal := newPalette(false)
	rows := testDailyRows() // 3 filas de octubre 2026 (jue, vie, sáb)
	m := buildWeekBalance("es", rows, pal)
	if len(m.Days) != 3 {
		t.Fatalf("days = %d", len(m.Days))
	}
	// Día 1: carga 2 kWh sobre dominio 8.
	d0 := m.Days[0]
	wantH := 2.0 / 8.0 * (weekH - weekTop - weekBottom)
	if math.Abs(d0.CargaH-wantH) > 0.01 {
		t.Fatalf("cargaH = %v (esperado %v)", d0.CargaH, wantH)
	}
	// Letras: 2026-10-01 jueves → J en es.
	if d0.Label != "J" {
		t.Fatalf("label = %q", d0.Label)
	}
	// Día 3 sin batería: barras a cero pero etiqueta presente.
	d2 := m.Days[2]
	if d2.CargaH != 0 || d2.DescargaH != 0 || d2.Label == "" {
		t.Fatalf("día sin batería = %+v", d2)
	}
	if len(m.TicksY) != 5 || m.TicksY[4].Label != "8" {
		t.Fatalf("ticksY = %+v", m.TicksY)
	}
	// Vacío.
	if e := buildWeekBalance("es", nil, pal); len(e.Days) != 0 {
		t.Fatal("sin filas no debería haber días")
	}
}
