package extensions

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngDataURLTest(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestChargerImageRejectsNonImagePayload(t *testing.T) {
	dir := t.TempDir()
	payload := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>"))

	if err := WriteChargerImageFile(dir, "inv1", payload); err == nil {
		t.Fatal("se guardo un data URL que no contiene una imagen decodificable")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "charger-images"))
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "inv1.") {
				t.Fatalf("se escribio el fichero %s con bytes no validados", e.Name())
			}
		}
	}
}

func TestChargerImageStoreIgnoresDeclaredMime(t *testing.T) {
	dir := t.TempDir()
	// PNG real anunciado como webp: el tipo guardado debe venir del decodificador,
	// no de la etiqueta del data URL.
	png := pngDataURLTest(t, 2, 2)
	lying := strings.Replace(png, "data:image/png;", "data:image/webp;", 1)

	if err := WriteChargerImageFile(dir, "inv2", lying); err == nil {
		t.Fatal("se acepto un data URL cuyo contenido no coincide con el tipo declarado")
	}
}

func TestChargerImageRejectsOversizedPixels(t *testing.T) {
	dir := t.TempDir()
	// Cabecera de 30000x30000 con muy pocos bytes: bomba de descompresion.
	img := image.NewGray(image.Rect(0, 0, 30000, 30000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Skipf("no se pudo construir el fixture: %v", err)
	}
	payload := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	if err := WriteChargerImageFile(dir, "inv3", payload); err == nil {
		t.Fatal("se acepto una imagen por encima del tope de pixeles")
	}
}

func TestChargerImageStoreWritesDecodedImage(t *testing.T) {
	dir := t.TempDir()
	if err := WriteChargerImageFile(dir, "inv4", pngDataURLTest(t, 3, 3)); err != nil {
		t.Fatalf("imagen valida rechazada: %v", err)
	}
	path := filepath.Join(dir, "charger-images", "inv4.png")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se guardo la imagen: %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("los bytes guardados no son una imagen decodificable: %v", err)
	}
}
