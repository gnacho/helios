package web

// Energy colors are INVARIABLE per energy type; only the theme changes the
// shade. The server emits CSS variables (var(--c-*)) instead of hex values:
// it cannot see the OS preference in the "auto" theme, so the light/dark
// choice is left to app.css, which defines every variable in both blocks.

// Palette resolves the semantic colors for templates and Datastar signals.
type Palette struct {
	Solar, Consumo, Bateria, Autoconsumo string
	RedCompra, RedVertido, Solis, Fox    string
}

func newPalette() Palette {
	return Palette{
		Solar:       "var(--c-solar)",
		Consumo:     "var(--c-consumo)",
		Bateria:     "var(--c-bateria)",
		Autoconsumo: "var(--c-autoconsumo)",
		RedCompra:   "var(--c-red-compra)",
		RedVertido:  "var(--c-red-vertido)",
		Solis:       "var(--c-solis)",
		Fox:         "var(--c-fox)",
	}
}

// InverterColor mirrors inverterColor(): solis/fox keep their semantic color,
// extra inverters cycle through the --c-invN variables by index.
func InverterColor(key string, p Palette, index int) string {
	switch key {
	case "solis":
		return p.Solis
	case "fox":
		return p.Fox
	}
	return "var(--c-inv" + string(rune('0'+index%6)) + ")"
}

// colorSoft is the translucent fill used by chips and strips: same hue at
// 12% over transparent (replaces the old 1F-hex alpha suffix).
func colorSoft(c string) string {
	return "color-mix(in srgb, " + c + " 12%, transparent)"
}
