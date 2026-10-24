package web

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"helios-server/internal/solar"
)

func testSeries() []solar.DayPoint {
	return []solar.DayPoint{
		{T: 0, Production: 0, Consumption: 0.3},
		{T: 300, Production: 0.5, Consumption: 0.4},
		{T: 600, Production: 2.5, Consumption: 0.6, Solis: 1.5, Fox: 1.0},
		{T: 720, Production: 3.483, Consumption: 0.8, Solis: 2.2, Fox: 1.28},
		{T: 900, Production: 1.5, Consumption: 1.2},
		{T: 1200, Production: 0.2, Consumption: 1.0},
		{T: 1425, Production: 0, Consumption: 0.7},
	}
}

func TestBuildDayChartGeometry(t *testing.T) {
	m := BuildDayChart("es", testSeries(), 720, []string{"solis", "fox"}, map[string]string{"solis": "Solis", "fox": "Fox"})

	// yMax: max 3.483 + 0.3 → ceil 4 (mínimo 1).
	if m.YMax != 4 {
		t.Fatalf("yMax = %d (esperado 4)", m.YMax)
	}
	// Picos en los extremos del eje X (geometría propia de la gráfica).
	g := geomDashboard
	if x := g.x(0); x != plotLeft {
		t.Fatalf("x(0) = %v (esperado %v)", x, plotLeft)
	}
	if x := g.x(1440); math.Abs(x-(g.W-plotRight)) > 0.01 {
		t.Fatalf("x(1440) = %v (esperado %v)", x, g.W-plotRight)
	}
	// El máximo de la curva toca el techo del plot.
	if y := g.y(4, 4); math.Abs(y-plotTop) > 0.01 {
		t.Fatalf("y(yMax) = %v (esperado %v)", y, plotTop)
	}
	// Ahora redondeado a paso de 5.
	if m.Now != 720 {
		t.Fatalf("Now = %d", m.Now)
	}
	// Etiquetas del eje X: 9 ticks cada 3 h (1440 da la vuelta a 00:00).
	if len(m.TicksX) != 9 || m.TicksX[1].Label != "03:00" || m.TicksX[3].Label != "09:00" || m.TicksX[8].Label != "00:00" {
		t.Fatalf("ticks X mal: %+v", m.TicksX)
	}
	// Ticks Y: 0..yMax.
	if len(m.TicksY) != m.YMax+1 {
		t.Fatalf("ticks Y = %d (esperado %d)", len(m.TicksY), m.YMax+1)
	}
}

func TestDayChartPaths(t *testing.T) {
	// Serie densa (paso de 5 min) como la real, con now en el índice 2.
	dense := []solar.DayPoint{
		{T: 0, Production: 0, Consumption: 0.3},
		{T: 5, Production: 0.2, Consumption: 0.35},
		{T: 10, Production: 0.5, Consumption: 0.4, Solis: 0.3, Fox: 0.2},
		{T: 15, Production: 0.8, Consumption: 0.4, Solis: 0.5, Fox: 0.3},
		{T: 20, Production: 1.1, Consumption: 0.5, Solis: 0.7, Fox: 0.4},
		{T: 25, Production: 1.3, Consumption: 0.5, Solis: 0.8, Fox: 0.5},
		{T: 30, Production: 1.5, Consumption: 0.6, Solis: 0.9, Fox: 0.6},
	}
	// nowMin=10 → nowIdx=2: índices 0-2 pasado, 2-6 futuro.
	m := BuildDayChart("es", dense, 10, []string{"solis", "fox"}, nil)

	// Pasado: termina en t=600 (nowIdx=2 → índices 0..2 con t<=600).
	if !strings.Contains(m.ProdPastPath, "M ") {
		t.Fatal("prod past sin M")
	}
	if !strings.HasSuffix(m.ProdPastPath, "Z") {
		t.Fatal("el área del pasado no se cierra en la baseline")
	}
	// Futuro: comienza en t=10 (nowIdx=2).
	wantX := fmt.Sprintf("%.2f", geomDashboard.x(10))
	if !strings.Contains(m.ProdFuturePath, wantX) {
		t.Fatalf("el futuro no empieza en x=%s: %q", wantX, m.ProdFuturePath[:60])
	}
	// Inversores: paths de línea (sin cierre Z).
	for _, inv := range m.InvPaths {
		if strings.HasSuffix(inv.Path, "Z") {
			t.Fatalf("%s no debería cerrarse como área", inv.Key)
		}
		if !strings.Contains(inv.Path, "C ") && !strings.Contains(inv.Path, "L ") {
			t.Fatalf("%s sin curva", inv.Key)
		}
	}
	// Sin inversores conocidos → claves con var genérica.
	m2 := BuildDayChart("es", testSeries(), 600, []string{"rare"}, nil)
	if m2.InvPaths[0].Var != "var(--c-solar)" {
		t.Fatalf("var de inversor desconocido = %q", m2.InvPaths[0].Var)
	}
}

func TestDayChartDeterministic(t *testing.T) {
	a := BuildDayChart("es", testSeries(), 720, []string{"solis", "fox"}, nil)
	b := BuildDayChart("es", testSeries(), 720, []string{"solis", "fox"}, nil)
	if a.ProdPastPath != b.ProdPastPath || a.ConsPath != b.ConsPath || a.ProdFuturePath != b.ProdFuturePath {
		t.Fatal("paths no deterministas entre builds idénticas")
	}
	if len(a.InvPaths) != len(b.InvPaths) || a.InvPaths[0].Path != b.InvPaths[0].Path {
		t.Fatal("paths de inversor no deterministas")
	}
}

func TestMonotonePathSplitsGaps(t *testing.T) {
	pts := []cpoint{
		{geomDashboard.x(0), 10}, {geomDashboard.x(5), 12}, {geomDashboard.x(10), 15},
		// hueco de 15 min (3 buckets sin datos)
		{geomDashboard.x(25), 20}, {geomDashboard.x(30), 22},
	}
	p := monotonePath(pts)
	// Dos runs: 1 M inicial + 1 M tras el hueco.
	if got := strings.Count(p, " M "); got != 1 {
		t.Fatalf("esperado 1 hueco (2 runs), hay %d: %q", got, p)
	}
	if !strings.HasPrefix(p, "M ") {
		t.Fatal("no empieza con M")
	}
	if strings.Contains(p, "NaN") {
		t.Fatal("NaN en el path")
	}
}
