// Package web renders the HTML UI (templ + Datastar) when HELIOS_UI=templ.
// The JSON API is untouched: this package only adds page routes on top.
package web

import (
	"embed"
	"encoding/json"
	"strings"
)

//go:embed locales/es/translation.json locales/en/translation.json locales/zh-CN/translation.json
var localesFS embed.FS

// Languages lists the supported UI languages (must match the API's langRe).
var Languages = []string{"es", "en", "zh-CN"}

type catalog map[string]any

var catalogs = map[string]catalog{}

func init() {
	for _, lang := range Languages {
		raw, err := localesFS.ReadFile("locales/" + lang + "/translation.json")
		if err != nil {
			panic("web: missing i18n catalog: " + err.Error())
		}
		var c catalog
		if err := json.Unmarshal(raw, &c); err != nil {
			panic("web: invalid i18n catalog (" + lang + "): " + err.Error())
		}
		catalogs[lang] = c
	}
}

func normalizeLang(lang string) string {
	if _, ok := catalogs[lang]; ok {
		return lang
	}
	return "es"
}

// lookup walks a dot path ("nav.dashboard") inside the catalog.
func lookup(c catalog, key string) (string, bool) {
	var cur any = map[string]any(c)
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[part]
		if !ok {
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok
}

// T translates key into lang, falling back to es and then to the key itself
// (a visible key during the port is better than an empty string).
// An optional replacement map fills "{{name}}" placeholders.
func T(lang, key string, repl ...map[string]string) string {
	lang = normalizeLang(lang)
	s, ok := lookup(catalogs[lang], key)
	if !ok && lang != "es" {
		s, ok = lookup(catalogs["es"], key)
	}
	if !ok {
		return key
	}
	if len(repl) > 0 {
		for k, v := range repl[0] {
			s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		}
	}
	return s
}

// TP is T with i18next-style plural suffix selection: key_one when the
// catalog has it and count == 1 (zh-CN has no plural distinction), key_other
// otherwise. Mirrors the trees_one/trees_other and streakDays_* keys.
func TP(lang, key string, count int, repl map[string]string) string {
	if normalizeLang(lang) != "zh-CN" && count == 1 {
		if _, ok := lookup(catalogs[normalizeLang(lang)], key+"_one"); ok {
			return T(lang, key+"_one", repl)
		}
	}
	return T(lang, key+"_other", repl)
}
