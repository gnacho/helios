package web

// Charger image upload (form POST wrapper of PUT /api/charger/image/{key}).
// The React app rescaled the picture in a canvas; server-rendered there is no
// client JS, so the resize happens here (max 460 px high, like the React
// canvas) before handing the data URL to extensions.SaveChargerImage.

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg" // decode support for the upload accept list
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"helios-server/internal/extensions"
)

const (
	chargerImageMaxHeight = 460
	// Cota real de la subida. Va en el MaxBytesReader ANTES de leer nada:
	// PostFormValue dispara ParseMultipartForm y consume el cuerpo entero, asi
	// que poner el limite despues no limitaba nada.
	chargerImageMaxBytes = 8 << 20
	// Tope de pixeles: image.Decode reserva w*h*4 a partir de la cabecera, de
	// modo que un PNG de 300 bytes con IHDR de 30000x30000 pide 3,6 GB.
	chargerImageMaxPixels = 40_000_000
)

// handleChargerImageUpload stores a custom charger picture (multipart form).
func (u *UI) handleChargerImageUpload(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	username, userID, _ := u.currentUserActor(r)
	// El tope va antes de tocar el cuerpo: PostFormValue ya parsea el multipart.
	r.Body = http.MaxBytesReader(w, r.Body, chargerImageMaxBytes)
	if err := r.ParseMultipartForm(chargerImageMaxBytes); err != nil {
		ajustesRedirectFlag(w, r, "err", "charger-image")
		return
	}
	key := r.PostFormValue("charger")
	if !extensions.SanitizeImageKey(key) {
		http.Redirect(w, r, "/cargador", http.StatusSeeOther)
		return
	}
	// PRG back to the SAME charger tab (a photo upload from c2 used to jump
	// back to the first charger).
	back := "/cargador?tab=" + url.QueryEscape(key)
	fail := back + "&imgerr=1"
	f, _, err := r.FormFile("image")
	if err != nil {
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, chargerImageMaxBytes+1))
	if err != nil || len(raw) > chargerImageMaxBytes {
		log.Printf("[charger-image] fichero demasiado grande o ilegible")
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	if !imageSizeOK(raw) {
		log.Printf("[charger-image] imagen con demasiados pixeles, descartada")
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		log.Printf("[charger-image] decode %v", err)
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	// Photos exported with a lot of empty canvas (transparent padding, or a
	// plain white background) looked small inside the card: crop the border
	// first so the car uses the full height, then cap the height.
	dst := resizeToHeight(trimEmptyMargins(src), chargerImageMaxHeight)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	if ok, msg := u.Ext.SaveChargerImage(u.DataDir, key, dataURL); !ok {
		log.Printf("[charger-image] save: %s", msg)
		http.Redirect(w, r, fail, http.StatusSeeOther)
		return
	}
	if u.DB != nil {
		u.DB.Audit(username, userID, "charger.image", map[string]string{"charger": key})
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// bgTolerance is how far a pixel may stray from the border colour and still
// count as empty background (0-255 per channel).
const bgTolerance = 12

// trimEmptyMargins crops the uniform border around the subject: transparent
// pixels when the image has alpha, otherwise a plain colour taken from the
// corners (the usual car photo on white). It keeps 2% of padding and leaves
// the image untouched when the border is not even or when the crop would eat
// most of the picture.
func trimEmptyMargins(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 16 {
		return src
	}
	n := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(n, n.Bounds(), src, b.Min, draw.Src)

	// Alpha present anywhere means the padding (if any) is transparent.
	transparent := false
	for y := 0; y < h && !transparent; y++ {
		row := n.Pix[y*n.Stride:]
		for x := 0; x < w; x++ {
			if row[x*4+3] < 250 {
				transparent = true
				break
			}
		}
	}

	var bg [3]byte
	if !transparent {
		corners := [4][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}}
		i := corners[0][1]*n.Stride + corners[0][0]*4
		bg = [3]byte{n.Pix[i], n.Pix[i+1], n.Pix[i+2]}
		for _, c := range corners[1:] {
			j := c[1]*n.Stride + c[0]*4
			if !closeTo(n.Pix[j:j+3], bg) {
				return src // not a plain border, do not guess
			}
		}
	}

	inside := func(x, y int) bool {
		i := y*n.Stride + x*4
		if transparent {
			return n.Pix[i+3] > 8
		}
		return !closeTo(n.Pix[i:i+3], bg)
	}

	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !inside(x, y) {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < 0 {
		return src // nothing but border: leave it alone
	}

	cw, ch := maxX-minX+1, maxY-minY+1
	if cw < w/4 || ch < h/4 {
		return src // suspiciously small subject, keep the original
	}
	if cw >= w-2 && ch >= h-2 {
		return src // nothing to trim
	}

	// Only crop when there is real air around the subject (4% of the side, at
	// least 3 px). This also keeps the pass idempotent: the padding left by a
	// previous crop (2%) never triggers another crop.
	thX, thY := max(3, w/25), max(3, h/25)
	left, right := minX, w-1-maxX
	top, bottom := minY, h-1-maxY
	if left < thX && right < thX && top < thY && bottom < thY {
		return src
	}

	padX, padY := max(2, cw/50), max(2, ch/50)
	rect := image.Rect(minX-padX, minY-padY, maxX+1+padX, maxY+1+padY).Intersect(b)
	return n.SubImage(rect)
}

// closeTo reports whether an RGB pixel is within bgTolerance of the background.
func closeTo(pix []byte, bg [3]byte) bool {
	for i := 0; i < 3; i++ {
		d := int(pix[i]) - int(bg[i])
		if d < 0 {
			d = -d
		}
		if d > bgTolerance {
			return false
		}
	}
	return true
}

// imageSizeOK reads only the header and rejects decompression bombs before the
// decoder allocates width*height pixels.
func imageSizeOK(raw []byte) bool {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	return cfg.Width > 0 && cfg.Height > 0 && cfg.Width*cfg.Height <= chargerImageMaxPixels
}

// OptimizeStoredChargerImages re-crops the pictures already on disk. Uploads
// made from the React app were only scaled in the canvas, so they keep a lot
// of empty border and look small next to the actions. The crop is idempotent
// (trimEmptyMargins leaves images that already fit alone) and it never
// upscales, so running it on every start only touches the first time.
func (u *UI) OptimizeStoredChargerImages() {
	if u.DataDir == "" {
		return
	}
	dir := filepath.Join(u.DataDir, "charger-images")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".png" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		cfg, _, cerr := image.DecodeConfig(f)
		if cerr != nil || cfg.Width*cfg.Height > chargerImageMaxPixels {
			f.Close()
			continue
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			continue
		}
		src, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		dst := resizeToHeight(trimEmptyMargins(src), chargerImageMaxHeight)
		if dst.Bounds().Size() == src.Bounds().Size() {
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			log.Printf("[charger-image] optimize %s: %v", entry.Name(), err)
			continue
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			log.Printf("[charger-image] optimize %s: %v", entry.Name(), err)
			continue
		}
		log.Printf("[charger-image] %s optimized %dx%d -> %dx%d", entry.Name(),
			src.Bounds().Dx(), src.Bounds().Dy(), dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

// resizeToHeight scales img down to at most h pixels of height (never up).
func resizeToHeight(src image.Image, h int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	sh := b.Dy()
	if sh <= h {
		return src
	}
	nw := max(1, w*h/sh)
	dst := image.NewRGBA(image.Rect(0, 0, nw, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}
