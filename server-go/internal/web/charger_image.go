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
	"log"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"helios-server/internal/extensions"
)

const chargerImageMaxHeight = 460

// handleChargerImageUpload stores a custom charger picture (multipart form).
func (u *UI) handleChargerImageUpload(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	username, userID, _ := u.currentUserActor(r)
	key := r.PostFormValue("charger")
	if !extensions.SanitizeImageKey(key) {
		http.Redirect(w, r, "/cargador?imgerr=1", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		ajustesRedirectFlag(w, r, "err", "charger-image")
		return
	}
	f, _, err := r.FormFile("image")
	if err != nil {
		http.Redirect(w, r, "/cargador?imgerr=1", http.StatusSeeOther)
		return
	}
	defer f.Close()

	src, kind, err := image.Decode(f)
	if err != nil {
		log.Printf("[charger-image] decode %v", err)
		http.Redirect(w, r, "/cargador?imgerr=1", http.StatusSeeOther)
		return
	}
	_ = kind
	dst := resizeToHeight(src, chargerImageMaxHeight)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		http.Redirect(w, r, "/cargador?imgerr=1", http.StatusSeeOther)
		return
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	if ok, msg := u.Ext.SaveChargerImage(u.DataDir, key, dataURL); !ok {
		log.Printf("[charger-image] save: %s", msg)
		http.Redirect(w, r, "/cargador?imgerr=1", http.StatusSeeOther)
		return
	}
	if u.DB != nil {
		u.DB.Audit(username, userID, "charger.image", map[string]string{"charger": key})
	}
	http.Redirect(w, r, "/cargador", http.StatusSeeOther)
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
