package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerHasResourceLimits(t *testing.T) {
	srv := newHTTPServer(":8080", http.NewServeMux())

	if srv.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", srv.Addr, ":8080")
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, 5*time.Second)
	}
	if srv.ReadTimeout != 60*time.Second {
		t.Fatalf("ReadTimeout = %s, want %s", srv.ReadTimeout, 60*time.Second)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %s, want %s", srv.IdleTimeout, 60*time.Second)
	}
	if srv.MaxHeaderBytes != 64<<10 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 64<<10)
	}
	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0 for SSE", srv.WriteTimeout)
	}
}
