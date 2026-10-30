package api

import (
	"io"
	"net/http"
	"strings"
)

const (
	defaultRequestBodyLimit int64 = 1 << 20
	imageRequestBodyLimit   int64 = 12 << 20
)

func bodyLimitForPath(path string) int64 {
	if path == "/cargador/charger-image" || strings.HasPrefix(path, "/api/charger/image/") {
		return imageRequestBodyLimit
	}
	return defaultRequestBodyLimit
}

type bodyLimitReader struct {
	io.ReadCloser
	remaining int64
	limit     int64
	exceeded  bool
}

func (b *bodyLimitReader) Read(p []byte) (int, error) {
	if b.exceeded {
		return 0, &http.MaxBytesError{Limit: b.limit}
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	if int64(n) > b.remaining {
		n = int(b.remaining)
		b.remaining = 0
		b.exceeded = true
		return n, &http.MaxBytesError{Limit: b.limit}
	}
	b.remaining -= int64(n)
	return n, err
}

type bodyLimitWriter struct {
	http.ResponseWriter
	body        *bodyLimitReader
	wroteHeader bool
}

func (w *bodyLimitWriter) WriteHeader(code int) {
	if w.body.exceeded {
		code = http.StatusRequestEntityTooLarge
	}
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *bodyLimitWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *bodyLimitWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *bodyLimitWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func requestBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := bodyLimitForPath(r.URL.Path)
		if r.ContentLength > limit {
			r.Body.Close()
			w.Header().Set("Connection", "close")
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		body := &bodyLimitReader{ReadCloser: r.Body, remaining: limit, limit: limit}
		r.Body = body
		next.ServeHTTP(&bodyLimitWriter{ResponseWriter: w, body: body}, r)
	})
}
