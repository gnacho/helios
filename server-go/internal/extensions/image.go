package extensions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "golang.org/x/image/webp"
)

var imageDataRe = regexp.MustCompile(`^data:image/(png|jpeg|webp);base64,(.+)$`)

const chargerImageMax = 4 * 1024 * 1024

// Pixel ceiling shared with the web upload path; a header-only check happens
// before the decoder allocates width*height pixels.
const chargerImageMaxPixels = 40_000_000

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

// DecodeDataURLImage validates a base64 data URL, enforces a pixel budget and
// returns a re-encoded image plus its canonical extension. The declared media
// type must match the decoded format, so a mislabeled payload is rejected.
func DecodeDataURLImage(raw string, maxPixels int) ([]byte, string, error) {
	return decodeImage(raw, maxPixels)
}

// WriteChargerImageFile escribe el data URL como <key>.<ext> y borra las demás
// extensiones de esa clave. No toca la configuración del KV.
//
// The bytes are decoded and re-encoded: the data URL label is not trusted, so a
// text payload cannot land on disk disguised as an image. The pixel ceiling is
// checked from the header before decoding, which keeps a small PNG with a huge
// IHDR from allocating gigabytes.
func WriteChargerImageFile(dataDir, key, dataURL string) error {
	out, ext, err := decodeImage(dataURL, chargerImageMaxPixels)
	if err != nil {
		return err
	}
	dir := ChargerImageDir(dataDir)
	for _, e := range imageExts {
		f := filepath.Join(dir, key+"."+e)
		if _, err := os.Stat(f); err == nil {
			os.Remove(f)
		}
	}
	return os.WriteFile(filepath.Join(dir, key+"."+ext), out, 0o644)
}

// decodeImage turns a base64 data URL into a re-encoded image. It validates the
// payload before decoding: base64 must be valid, the decoded bytes must parse as
// an image, the pixel budget must hold, and the declared media type must match
// the detected format. Re-encoding never trusts the original bytes.
func decodeImage(raw string, maxPixels int) ([]byte, string, error) {
	m := imageDataRe.FindStringSubmatch(raw)
	if m == nil {
		return nil, "", errors.New("formato de imagen invalido")
	}
	buf, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil || len(buf) == 0 {
		return nil, "", errors.New("imagen vacia")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return nil, "", errors.New("formato de imagen invalido")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, "", errors.New("imagen con demasiados pixeles")
	}
	declared := m[1]
	if declared == "jpeg" {
		declared = "jpg"
	}
	if format != declared {
		return nil, "", errors.New("el tipo declarado no coincide con la imagen")
	}
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", errors.New("formato de imagen invalido")
	}
	var out bytes.Buffer
	ext := "png"
	switch format {
	case "jpg", "jpeg":
		ext = "jpg"
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", errors.New("no se pudo recodificar la imagen")
		}
	default:
		if err := png.Encode(&out, img); err != nil {
			return nil, "", errors.New("no se pudo recodificar la imagen")
		}
	}
	return out.Bytes(), ext, nil
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
	if err := WriteChargerImageFile(dataDir, key, dataURL); err != nil {
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
