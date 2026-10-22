package extensions

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var imageDataRe = regexp.MustCompile(`^data:image/(png|jpeg|webp);base64,(.+)$`)

const chargerImageMax = 4 * 1024 * 1024

var imageMimeByExt = map[string]string{
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"webp": "image/webp",
}

var imageExts = []string{"png", "jpg", "webp"}

func ChargerImageDir(dataDir string) string {
	dir := filepath.Join(dataDir, "charger-images")
	os.MkdirAll(dir, 0o755)
	return dir
}

// SetChargerImageURL actualiza imageUrl del cargador en kv y re-resuelve.
func (e *Ext) SetChargerImageURL(key, url string) {
	raw, err := e.DB.KVGet("extensions_config")
	if err != nil || raw == "" {
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return
	}
	carCharger, _ := cfg["carCharger"].(map[string]any)
	if carCharger == nil {
		return
	}
	list, _ := carCharger["chargers"].([]any)
	for _, item := range list {
		ch, _ := item.(map[string]any)
		if ch == nil {
			continue
		}
		if k, _ := ch["key"].(string); k == key {
			ch["imageUrl"] = url
			if out, err := json.Marshal(cfg); err == nil {
				e.DB.KVSet("extensions_config", string(out))
				e.Store.Set(NormalizeConfig(cfg, GenericExtensions))
			}
			return
		}
	}
}

// SaveChargerImage guarda la imagen (data URL) y apunta imageUrl al endpoint.
func (e *Ext) SaveChargerImage(dataDir, key, dataURL string) (bool, string) {
	m := imageDataRe.FindStringSubmatch(dataURL)
	if m == nil {
		return false, "formato de imagen invalido"
	}
	buf, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil || len(buf) == 0 {
		return false, "imagen vacia"
	}
	if len(buf) > chargerImageMax {
		return false, "imagen demasiado grande (max 4 MB)"
	}

	dir := ChargerImageDir(dataDir)
	for _, ext := range imageExts {
		f := filepath.Join(dir, key+"."+ext)
		if _, err := os.Stat(f); err == nil {
			os.Remove(f)
		}
	}

	ext := "png"
	switch m[1] {
	case "jpeg":
		ext = "jpg"
	case "webp":
		ext = "webp"
	}

	if err := os.WriteFile(filepath.Join(dir, key+"."+ext), buf, 0o644); err != nil {
		return false, "no se pudo guardar la imagen"
	}
	e.SetChargerImageURL(key, "/api/charger/image/"+key)
	return true, ""
}

// ClearChargerImage borra ficheros y limpia imageUrl.
func (e *Ext) ClearChargerImage(dataDir, key string) {
	dir := ChargerImageDir(dataDir)
	for _, ext := range imageExts {
		f := filepath.Join(dir, key+"."+ext)
		if _, err := os.Stat(f); err == nil {
			os.Remove(f)
		}
	}
	e.SetChargerImageURL(key, "")
}

// FindChargerImage devuelve ruta y mime de la imagen, o ("", "").
func FindChargerImage(dataDir, key string) (string, string) {
	dir := ChargerImageDir(dataDir)
	for _, ext := range imageExts {
		f := filepath.Join(dir, key+"."+ext)
		if _, err := os.Stat(f); err == nil {
			return f, imageMimeByExt[ext]
		}
	}
	return "", ""
}

// SanitizeImageKey valida la key para rutas de imagen.
func SanitizeImageKey(key string) bool {
	if key == "" || len(key) > 20 {
		return false
	}
	return keyRe.MatchString(strings.ToLower(key))
}
