package web

// Energy palette ported from app/src/lib/colors.ts. Colors are INVARIABLE per
// energy type; only the theme (light/dark) changes the shade.

type shade struct{ Light, Dark string }

var energyColors = map[string]shade{
	"solar":       {"#F59E0B", "#FBBF24"},
	"consumo":     {"#3B82F6", "#60A5FA"},
	"bateria":     {"#10B981", "#34D399"},
	"autoconsumo": {"#10B981", "#34D399"},
	"redCompra":   {"#F43F5E", "#FB7185"},
	"redVertido":  {"#06B6D4", "#22D3EE"},
	"solis":       {"#0EA5E9", "#38BDF8"},
	"fox":         {"#8B5CF6", "#A78BFA"},
}

// extraInverterColors is the cyclic palette for inverters beyond solis/fox.
var extraInverterColors = []shade{
	{"#10B981", "#34D399"},
	{"#F43F5E", "#FB7185"},
	{"#06B6D4", "#22D3EE"},
	{"#F59E0B", "#FBBF24"},
	{"#EC4899", "#F472B6"},
	{"#84CC16", "#A3E635"},
}

// Palette resolves the semantic colors for a theme.
type Palette struct {
	Dark                              bool
	Solar, Consumo, Bateria           string
	RedCompra, RedVertido, Solis, Fox string
	Autoconsumo                       string
}

func newPalette(dark bool) Palette {
	pick := func(k string) string {
		s := energyColors[k]
		if dark {
			return s.Dark
		}
		return s.Light
	}
	return Palette{
		Dark:        dark,
		Solar:       pick("solar"),
		Consumo:     pick("consumo"),
		Bateria:     pick("bateria"),
		RedCompra:   pick("redCompra"),
		RedVertido:  pick("redVertido"),
		Solis:       pick("solis"),
		Fox:         pick("fox"),
		Autoconsumo: pick("autoconsumo"),
	}
}

// InverterColor mirrors inverterColor(): solis/fox keep their semantic color,
// extra inverters cycle through the extra palette by index.
func InverterColor(key string, p Palette, index int) string {
	switch key {
	case "solis":
		return p.Solis
	case "fox":
		return p.Fox
	}
	s := extraInverterColors[index%len(extraInverterColors)]
	if p.Dark {
		return s.Dark
	}
	return s.Light
}
