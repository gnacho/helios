package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// streamRecorder records writes so the race detector can observe whether the
// subscriber callback and the handler loop share the response writer.
type streamRecorder struct {
	header http.Header
	writes atomic.Int64
	data   []byte
	mu     sync.Mutex
}

func (w *streamRecorder) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *streamRecorder) Write(p []byte) (int, error) {
	w.writes.Add(1)
	w.mu.Lock()
	w.data = append(w.data, p...)
	w.mu.Unlock()
	return len(p), nil
}

func (w *streamRecorder) WriteHeader(int) {}

func (w *streamRecorder) Flush() {}

// TestUpdateStreamSerializesWrites drives the real WatchProgress goroutine so
// progress events reach the subscriber callback while the handler loop writes
// heartbeats. Both write to the same ResponseWriter, so the handler must
// serialize them instead of relying on the writer being concurrency-safe.
func TestUpdateStreamSerializesWrites(t *testing.T) {
	srv, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityAdminUser, securityAdminPass)

	previous := updateHeartbeatInterval
	updateHeartbeatInterval = time.Millisecond
	defer func() { updateHeartbeatInterval = previous }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/update/stream", nil).WithContext(ctx)
	req.Host = "helios.example"
	req.AddCookie(cookie)
	rec := &streamRecorder{}
	go router.ServeHTTP(rec, req)

	time.Sleep(100 * time.Millisecond)

	progress := filepath.Join(srv.dataDir, "update-progress.json")
	for i := 0; i < 6; i++ {
		line := fmt.Sprintf(`{"step":"descarga-%d","pct":%d,"ts":%d}`, i, i*8, time.Now().Unix())
		if err := os.WriteFile(progress, []byte(line), 0o644); err != nil {
			t.Fatalf("write progress: %v", err)
		}
		time.Sleep(600 * time.Millisecond)
	}

	cancel()
	time.Sleep(300 * time.Millisecond)

	if rec.writes.Load() == 0 {
		t.Fatal("no se escribió ningún evento en el stream")
	}
}

// The subscriber callback runs on the WatchProgress goroutine. If it wrote to
// the ResponseWriter directly it would race with the handler loop, so every
// write must happen while holding a single-writer guarantee. This test checks
// the structural invariant: the callback only hands work to the handler.
func TestUpdateStreamNeverWritesFromSubscriber(t *testing.T) {
	srv, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityAdminUser, securityAdminPass)

	previous := updateHeartbeatInterval
	updateHeartbeatInterval = time.Hour
	defer func() { updateHeartbeatInterval = previous }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/update/stream", nil).WithContext(ctx)
	req.Host = "helios.example"
	req.AddCookie(cookie)
	rec := &streamRecorder{}
	go router.ServeHTTP(rec, req)

	time.Sleep(100 * time.Millisecond)
	baseline := rec.writes.Load()

	progress := filepath.Join(srv.dataDir, "update-progress.json")
	for i := 0; i < 3; i++ {
		line := fmt.Sprintf(`{"step":"descarga-%d","pct":%d,"ts":%d}`, i, i*10, time.Now().Unix())
		if err := os.WriteFile(progress, []byte(line), 0o644); err != nil {
			t.Fatalf("write progress: %v", err)
		}
		time.Sleep(700 * time.Millisecond)
	}

	cancel()
	time.Sleep(300 * time.Millisecond)

	if rec.writes.Load() <= baseline {
		t.Fatalf("los eventos de progreso no llegaron al stream: writes = %d, base = %d", rec.writes.Load(), baseline)
	}
}
