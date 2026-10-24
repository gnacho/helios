package web

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filled draws a coloured rectangle on top of the given canvas.
func filled(w, h int, bg color.NRGBA, rect image.Rectangle, fg color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, fg)
		}
	}
	return img
}

func TestTrimEmptyMarginsTransparentPadding(t *testing.T) {
	// 400x300 canvas with the subject only in the middle third.
	src := filled(400, 300, color.NRGBA{0, 0, 0, 0}, image.Rect(100, 100, 300, 200), color.NRGBA{200, 30, 30, 255})
	dst := trimEmptyMargins(src)
	b := dst.Bounds()
	if b.Dx() > 220 || b.Dy() > 120 {
		t.Fatalf("no recortó el relleno transparente: %v", b)
	}
	if b.Dx() < 200 || b.Dy() < 100 {
		t.Fatalf("recortó de más: %v", b)
	}
}

func TestTrimEmptyMarginsPlainBackground(t *testing.T) {
	// Photo on a white background: the white border goes away.
	white := color.NRGBA{255, 255, 255, 255}
	src := filled(600, 400, white, image.Rect(60, 40, 540, 360), color.NRGBA{20, 90, 200, 255})
	dst := trimEmptyMargins(src)
	b := dst.Bounds()
	if b.Dx() > 520 || b.Dy() > 360 {
		t.Fatalf("no recortó el fondo liso: %v", b)
	}
	if b.Dx() < 480 || b.Dy() < 320 {
		t.Fatalf("recortó de más: %v", b)
	}
}

func TestTrimEmptyMarginsKeepsFullFramePhoto(t *testing.T) {
	// No uniform border (different corner colours): must stay untouched.
	src := filled(400, 300, color.NRGBA{10, 20, 30, 255}, image.Rect(50, 50, 350, 250), color.NRGBA{200, 30, 30, 255})
	src.SetNRGBA(399, 0, color.NRGBA{250, 250, 10, 255})
	dst := trimEmptyMargins(src)
	if dst.Bounds() != src.Bounds() {
		t.Fatalf("no debería recortar: %v", dst.Bounds())
	}
}

func TestTrimEmptyMarginsKeepsUniformImage(t *testing.T) {
	// All-white picture: nothing to keep, do not crop to nothing.
	src := filled(400, 300, color.NRGBA{255, 255, 255, 255}, image.Rect(0, 0, 0, 0), color.NRGBA{0, 0, 0, 255})
	dst := trimEmptyMargins(src)
	if dst.Bounds() != src.Bounds() {
		t.Fatalf("imagen vacía recortada: %v", dst.Bounds())
	}
}

func TestTrimEmptyMarginsKeepsSmallSubject(t *testing.T) {
	// A tiny subject in a huge canvas would lose the composition: keep it.
	src := filled(600, 400, color.NRGBA{255, 255, 255, 255}, image.Rect(290, 190, 310, 210), color.NRGBA{0, 0, 0, 255})
	dst := trimEmptyMargins(src)
	if dst.Bounds() != src.Bounds() {
		t.Fatalf("sujeto diminuto recortado: %v", dst.Bounds())
	}
}

func TestTrimEmptyMarginsIsIdempotent(t *testing.T) {
	// Cropping twice must not shrink the picture again.
	src := filled(600, 400, color.NRGBA{255, 255, 255, 255}, image.Rect(60, 40, 540, 360), color.NRGBA{20, 90, 200, 255})
	once := trimEmptyMargins(src)
	twice := trimEmptyMargins(once)
	if once.Bounds().Size() != twice.Bounds().Size() {
		t.Fatalf("no es idempotente: %v -> %v", once.Bounds().Size(), twice.Bounds().Size())
	}
}

func TestOptimizeStoredChargerImages(t *testing.T) {
	dir := t.TempDir()
	imgDir := dir + "/charger-images"
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A 600x400 picture whose subject only fills the middle: 60 px of border.
	src := filled(600, 400, color.NRGBA{0, 0, 0, 0}, image.Rect(60, 40, 540, 360), color.NRGBA{20, 90, 200, 255})
	f, err := os.Create(imgDir + "/main.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	u := &UI{DataDir: dir}
	u.OptimizeStoredChargerImages()

	out, err := os.Open(imgDir + "/main.png")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	got, _, err := image.Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dy() >= 400 {
		t.Fatalf("no recortó la foto guardada: %v", got.Bounds())
	}

	// Second run is a no-op (same size on disk).
	before, _ := os.Stat(imgDir + "/main.png")
	u.OptimizeStoredChargerImages()
	after, _ := os.Stat(imgDir + "/main.png")
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("la segunda pasada reescribió el fichero")
	}
}

// craftPNGHeader builds a PNG whose IHDR declares huge dimensions with almost
// no pixel data: enough for DecodeConfig, lethal for a decoder that trusts it.
func craftPNGHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	chunk := func(kind string, data []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(data)))
		buf.Write(l[:])
		buf.WriteString(kind)
		buf.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(kind))
		crc.Write(data)
		var c [4]byte
		binary.BigEndian.PutUint32(c[:], crc.Sum32())
		buf.Write(c[:])
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // RGB
	chunk("IHDR", ihdr)
	chunk("IDAT", []byte{0x78, 0x9c, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01})
	chunk("IEND", nil)
	return buf.Bytes()
}

func TestImageSizeOKRejectsDecompressionBomb(t *testing.T) {
	if imageSizeOK(craftPNGHeader(30000, 30000)) {
		t.Fatal("aceptó una imagen de 900 MPx")
	}
	// Una foto normal sí pasa.
	small := &bytes.Buffer{}
	if err := png.Encode(small, filled(400, 300, color.NRGBA{0, 0, 0, 0}, image.Rect(10, 10, 390, 290), color.NRGBA{1, 2, 3, 255})); err != nil {
		t.Fatal(err)
	}
	if !imageSizeOK(small.Bytes()) {
		t.Fatal("rechazó una imagen normal")
	}
}

// TestChargerImageUploadRejectsBomb: el tope de píxeles corta antes de que el
// decodificador reserve 3,6 GB.
func TestChargerImageUploadRejectsBomb(t *testing.T) {
	u, router := chargerTestUI(t)
	u.DataDir = t.TempDir()
	cookie := sessionCookie(t, doLogin(t, router, "nacho", "password1"))

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("charger", "main")
	fw, err := mw.CreateFormFile("image", "bomb.png")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(craftPNGHeader(30000, 30000))
	mw.Close()

	req := httptest.NewRequest("POST", "/cargador/charger-image", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "imgerr=1") {
		t.Fatalf("debería rechazar la bomba, Location = %q", loc)
	}
	if _, err := os.Stat(filepath.Join(u.DataDir, "charger-images", "main.png")); err == nil {
		t.Fatal("guardó la imagen bomba")
	}
}

// TestChargerImageUploadSavesNormal: la ruta buena sigue funcionando.
func TestChargerImageUploadSavesNormal(t *testing.T) {
	u, router := chargerTestUI(t)
	u.DataDir = t.TempDir()
	cookie := sessionCookie(t, doLogin(t, router, "nacho", "password1"))

	img := &bytes.Buffer{}
	if err := png.Encode(img, filled(500, 400, color.NRGBA{0, 0, 0, 0}, image.Rect(20, 20, 480, 380), color.NRGBA{20, 90, 200, 255})); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("charger", "main")
	fw, _ := mw.CreateFormFile("image", "foto.png")
	fw.Write(img.Bytes())
	mw.Close()

	req := httptest.NewRequest("POST", "/cargador/charger-image", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "tab=main") || strings.Contains(loc, "imgerr") {
		t.Fatalf("no guardó: Location = %q", loc)
	}
	if _, err := os.Stat(filepath.Join(u.DataDir, "charger-images", "main.png")); err != nil {
		t.Fatalf("falta la imagen guardada: %v", err)
	}
}
