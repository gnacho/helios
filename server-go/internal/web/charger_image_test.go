package web

import (
	"image"
	"image/color"
	"image/png"
	"os"
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
