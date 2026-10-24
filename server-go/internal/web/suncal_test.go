package web

import (
	"testing"
	"time"
)

// Madrid 2026-10-24: SunCalc (the React app library) gives 08:35 / 19:22.
// La zona va fija (CEST, +02:00) para que el test no dependa de la zona del
// equipo: en CI el reloj corre en UTC y el amanecer salía a las 06:36.
func TestSunTimesMadrid(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)
	day := time.Date(2026, 10, 24, 12, 0, 0, 0, cest)
	rise, set, ok := SunTimes(day, 40.4168, -3.7038)
	if !ok {
		t.Fatal("sin horas de sol")
	}
	if rise < 8*60+32 || rise > 8*60+38 {
		t.Fatalf("amanecer = %02d:%02d (esperado 08:35)", rise/60, rise%60)
	}
	if set < 19*60+19 || set > 19*60+25 {
		t.Fatalf("ocaso = %02d:%02d (esperado 19:22)", set/60, set%60)
	}
}

func TestSunTimesPolarNight(t *testing.T) {
	// Tromsø in December: no sunrise.
	day := time.Date(2026, 12, 21, 12, 0, 0, 0, time.FixedZone("CET", 1*60*60))
	if _, _, ok := SunTimes(day, 69.6492, 18.9553); ok {
		t.Fatal("esperaba día/noche polar sin horas")
	}
}
