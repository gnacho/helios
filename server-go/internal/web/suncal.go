package web

import (
	"math"
	"time"
)

// SunCalc port (the JS library the React app used): sunrise/sunset from
// coordinates. All the intermediate values are the same as in SunCalc so the
// minutes match the original to the minute.

const (
	sunRad   = math.Pi / 180
	sunJ0    = 0.0009
	sunJ1970 = 2440588.0
	sunJ2000 = 2451545.0
	sunDayMs = 86400000.0
	sunOblq  = 23.4397 // obliquity of the Earth
)

// sunToDays is SunCalc's toDays(): days since J2000.
func sunToDays(t time.Time) float64 {
	return float64(t.UTC().UnixMilli())/sunDayMs - 0.5 + sunJ1970 - sunJ2000
}

// sunFromJulian converts a Julian day back to a time.
func sunFromJulian(j float64) time.Time {
	ms := int64((j + 0.5 - sunJ1970) * sunDayMs)
	return time.UnixMilli(ms).UTC()
}

func sunDeclination(l float64) float64 {
	e := sunRad * sunOblq
	return math.Asin(math.Sin(0)*math.Cos(e) + math.Cos(0)*math.Sin(e)*math.Sin(l))
}

func sunSolarMeanAnomaly(d float64) float64 {
	return sunRad * (357.5291 + 0.98560028*d)
}

func sunEclipticLongitude(m float64) float64 {
	c := sunRad * (1.9148*math.Sin(m) + 0.02*math.Sin(2*m) + 0.0003*math.Sin(3*m))
	p := sunRad * 102.9372
	return m + c + p + math.Pi
}

// SunTimes returns sunrise/sunset as minutes of the local day. ok is false for
// polar day or night (SunCalc yields NaN there).
func SunTimes(t time.Time, lat, lon float64) (sunrise, sunset int, ok bool) {
	d := sunToDays(t)
	lw := sunRad * -lon
	phi := sunRad * lat

	n := math.Round(d - sunJ0 - lw/(2*math.Pi))
	ds := sunJ0 + lw/(2*math.Pi) + n
	m := sunSolarMeanAnomaly(ds)
	l := sunEclipticLongitude(m)
	dec := sunDeclination(l)

	jNoon := sunJ2000 + ds + 0.0053*math.Sin(m) - 0.0069*math.Sin(2*l)
	cosW := (math.Sin(-0.833*sunRad) - math.Sin(phi)*math.Sin(dec)) / (math.Cos(phi) * math.Cos(dec))
	if math.IsNaN(cosW) || cosW < -1 || cosW > 1 {
		return 0, 0, false
	}
	w := math.Acos(cosW)
	jSet := sunJ2000 + (sunJ0 + (w+lw)/(2*math.Pi) + n) + 0.0053*math.Sin(m) - 0.0069*math.Sin(2*l)
	jRise := jNoon - (jSet - jNoon)

	rise := sunFromJulian(jRise).In(t.Location())
	set := sunFromJulian(jSet).In(t.Location())
	return rise.Hour()*60 + rise.Minute(), set.Hour()*60 + set.Minute(), true
}
