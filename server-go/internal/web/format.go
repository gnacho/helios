package web

import (
	"fmt"
	"math"
	"strings"
)

// Number formatting ported from app/src/lib/format.ts (Intl es-ES / en-US /
// zh-CN). Display strings are built server-side so Datastar signals carry
// ready-to-render text and no formatting logic lives in the browser.

// numLocale mirrors numLocale() from app/src/i18n.
func numLocale(lang string) string {
	switch normalizeLang(lang) {
	case "es":
		return "es-ES"
	default:
		return "en-US"
	}
}

// formatNumber formats with fixed decimals using the locale decimal mark.
func formatNumber(lang string, v float64, decimals int) string {
	sep := ","
	if numLocale(lang) != "es-ES" {
		sep = "."
	}
	s := fmt.Sprintf("%.*f", decimals, v)
	if sep == "," {
		s = strings.ReplaceAll(s, ".", ",")
	}
	return s
}

// fmtKw mirrors fmtKw(): always kW with 2 decimals; dash for invalid values.
func fmtKw(lang string, kw *float64) string {
	if kw == nil || math.IsNaN(*kw) || math.IsInf(*kw, 0) {
		return "—"
	}
	return formatNumber(lang, *kw, 2)
}

// fmtKwVal is fmtKw for non-optional values.
func fmtKwVal(lang string, kw float64) string {
	return fmtKw(lang, &kw)
}

// fmtEnergy mirrors fmtEnergy(): kWh with 1 decimal.
func fmtEnergy(lang string, kwh float64) string {
	return formatNumber(lang, kwh, 1)
}

// fmtInt formats with no decimals.
func fmtInt(lang string, v float64) string {
	return formatNumber(lang, math.Round(v), 0)
}

// fmtEuro mirrors fmtEuro(): 2 decimals + the euro sign.
func fmtEuro(lang string, v float64) string {
	return formatNumber(lang, v, 2) + " €"
}

// fmtPct mirrors fmtPct(): rounded integer percent (no sign).
func fmtPct(lang string, v float64) string {
	return fmtInt(lang, v)
}

// fmtTime mirrors fmtTime(): HH:MM from minutes since midnight.
func fmtTime(minutes int) string {
	m := ((minutes % 1440) + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}
