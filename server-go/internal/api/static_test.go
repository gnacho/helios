package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticFilesServesSPA(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>helios</html>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "app.css"), []byte("body{}"), 0o644)

	static := NewStaticFiles(dir)

	cases := []struct {
		path       string
		wantStatus int
		wantBody   string
		wantCache  string
	}{
		{"/", 200, "<html>helios</html>", "no-cache"},
		{"/ajustes/topologia", 200, "<html>helios</html>", "no-cache"},
		{"/assets/app.css", 200, "body{}", "max-age=31536000"},
		{"/sw.js", 404, "", ""}, // no existe -> fallback intenta index (404 porque index? no: fallback sirve index)
	}
	// sw.js no existe en disco; el fallback sirve index.html con 200.
	cases[3].wantStatus = 200
	cases[3].wantBody = "<html>helios</html>"

	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		rec := httptest.NewRecorder()
		static.ServeHTTP(rec, req)

		body := rec.Body.String()
		if rec.Code != c.wantStatus {
			t.Fatalf("%s: status = %d (esperado %d), body=%s", c.path, rec.Code, c.wantStatus, body)
		}
		if c.wantBody != "" && body != c.wantBody {
			t.Fatalf("%s: body = %q (esperado %q)", c.path, body, c.wantBody)
		}
		if c.wantCache != "" {
			cc := rec.Header().Get("Cache-Control")
			if !strings.Contains(cc, c.wantCache) {
				t.Fatalf("%s: cache-control = %q (esperado contener %q)", c.path, cc, c.wantCache)
			}
		}
	}
}

func TestStaticFilesBlocksTraversal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o644)
	static := NewStaticFiles(dir)

	req := httptest.NewRequest("GET", "/../etc/passwd", nil)
	req.URL.Path = "/../etc/passwd"
	rec := httptest.NewRecorder()
	static.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (esperado 404)", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if strings.Contains(string(body), "root:") {
		t.Fatal("¡filtró /etc/passwd!")
	}
}
