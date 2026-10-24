package web

import (
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// municipios.json is the same dataset the React autocomplete used
// (app/public/municipios.json, copied into the embedded assets).
type municipio struct {
	Name    string
	Prov    string
	Lat     float64
	Lon     float64
	Display string
}

var (
	municipiosOnce sync.Once
	municipiosList []municipio
)

func loadMunicipios() []municipio {
	municipiosOnce.Do(func() {
		raw, err := fs.ReadFile(assetsEmbed, "assets/municipios.json")
		if err != nil {
			return
		}
		var rows [][]any
		if err := json.Unmarshal(raw, &rows); err != nil {
			return
		}
		out := make([]municipio, 0, len(rows))
		for _, r := range rows {
			if len(r) < 4 {
				continue
			}
			name, _ := r[0].(string)
			prov, _ := r[1].(string)
			lat, _ := r[2].(float64)
			lon, _ := r[3].(float64)
			display := name
			if prov != "" {
				display = name + " (" + prov + ")"
			}
			out = append(out, municipio{Name: name, Prov: prov, Lat: lat, Lon: lon, Display: display})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Display < out[j].Display })
		municipiosList = out
	})
	return municipiosList
}

// normText strips accents and lowercases, like the React normText().
func normText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case 'á', 'à', 'ä', 'â':
			b.WriteRune('a')
		case 'é', 'è', 'ë', 'ê':
			b.WriteRune('e')
		case 'í', 'ì', 'ï', 'î':
			b.WriteRune('i')
		case 'ó', 'ò', 'ö', 'ô':
			b.WriteRune('o')
		case 'ú', 'ù', 'ü', 'û':
			b.WriteRune('u')
		case 'ñ':
			b.WriteRune('n')
		case 'ç':
			b.WriteRune('c')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// searchMunicipios returns up to n matches (prefix first, then contains).
func searchMunicipios(q string, n int) []municipio {
	if n <= 0 {
		n = 8
	}
	qn := normText(q)
	if len(qn) < 2 {
		return nil
	}
	list := loadMunicipios()
	var starts, contains []municipio
	for _, m := range list {
		dn := normText(m.Display)
		if strings.HasPrefix(dn, qn) {
			starts = append(starts, m)
		} else if strings.Contains(dn, qn) {
			contains = append(contains, m)
		}
		if len(starts) >= n {
			break
		}
	}
	out := append(starts, contains...)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// findMunicipio resolves a saved location string back to coordinates.
func findMunicipio(display string) (municipio, bool) {
	dn := normText(display)
	if dn == "" {
		return municipio{}, false
	}
	for _, m := range loadMunicipios() {
		if normText(m.Display) == dn {
			return m, true
		}
	}
	for _, m := range loadMunicipios() {
		if normText(m.Name) == dn {
			return m, true
		}
	}
	return municipio{}, false
}
