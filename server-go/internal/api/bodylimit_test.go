package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitForPath(t *testing.T) {
	cases := []struct {
		path string
		want int64
	}{
		{"/api/auth/login", defaultRequestBodyLimit},
		{"/api/config", defaultRequestBodyLimit},
		{"/", defaultRequestBodyLimit},
		{"/api/charger/image/main", imageRequestBodyLimit},
		{"/cargador/charger-image", imageRequestBodyLimit},
	}
	for _, tc := range cases {
		if got := bodyLimitForPath(tc.path); got != tc.want {
			t.Errorf("bodyLimitForPath(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}

func TestRequestBodyLimitAllowsImagePathAboveDefault(t *testing.T) {
	var read int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		read = n
		w.WriteHeader(http.StatusOK)
	})
	handler := requestBodyLimit(next)

	body := bytes.Repeat([]byte("a"), 2<<20)
	req := httptest.NewRequest(http.MethodPut, "/api/charger/image/main", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if read != int64(len(body)) {
		t.Fatalf("read = %d, want %d", read, len(body))
	}
}

func TestRequestBodyLimitFlushDoesNotWriteHeaderTwice(t *testing.T) {
	var logged bytes.Buffer
	handler := requestBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		io.WriteString(w, "data: ok\n\n")
	}))

	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ErrorLog = log.New(&logged, "", 0)
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ui/stream")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(string(body), "data: ok") {
		t.Fatalf("body = %q, want the event", body)
	}
	if strings.Contains(logged.String(), "superfluous") {
		t.Fatalf("flushing before the first write logged a superfluous WriteHeader: %s", logged.String())
	}
}
