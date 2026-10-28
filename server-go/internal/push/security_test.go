package push

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func TestEndpointValidationRejectsNonPublicTargets(t *testing.T) {
	bad := []string{
		"http://push.example/1",
		"https://user:pass@push.example/1",
		"https://127.0.0.1/1",
		"https://[::1]/1",
		"https://10.1.2.3/1",
		"https://192.168.1.5/1",
		"https://172.16.0.9/1",
		"https://169.254.169.254/latest/meta-data",
		"https://100.64.1.1/1",
		"https://0.0.0.0/1",
		"https://push.local/1",
		"https:///no-host",
		"not a url",
	}
	for _, endpoint := range bad {
		if err := ValidateEndpoint(endpoint); err == nil {
			t.Errorf("%q debería rechazarse", endpoint)
		}
	}

	good := []string{
		"https://push.example/1",
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/xyz",
	}
	for _, endpoint := range good {
		if err := ValidateEndpoint(endpoint); err != nil {
			t.Errorf("%q debería aceptarse: %v", endpoint, err)
		}
	}
}

func TestSendUsesTimeoutClientAndNoRedirects(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	checks := newOptionsGuard(t)

	s.SetSendFn(checks.fn)

	if err := d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p256", "auth", "ua", time.Now().UnixMilli()); err != nil {
		t.Fatalf("sub: %v", err)
	}

	s.NotifyUsers([]string{uid}, "bateria_baja", Datos{"soc": 5.0}, Options{})

	checks.assert(t)
}

func TestSendSkipsStoredInsecureEndpoint(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)

	var called atomic.Int32
	s.SetSendFn(func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
		called.Add(1)
		return 201, nil
	})

	for _, endpoint := range []string{"http://push.example/1", "https://127.0.0.1/1", "https://169.254.169.254/x"} {
		if err := d.UpsertPushSubscription("s-"+endpoint, uid, endpoint, "p256", "auth", "ua", time.Now().UnixMilli()); err != nil {
			t.Fatalf("sub: %v", err)
		}
	}

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{"soc": 5.0}, Options{})
	if called.Load() != 0 {
		t.Fatalf("se intentó enviar a %d endpoints no públicos", called.Load())
	}
	if res.Fallidos != 3 {
		t.Fatalf("fallidos = %d, esperado 3", res.Fallidos)
	}
}

func TestNotifyAllDoesNotSpawnUnboundedWorkers(t *testing.T) {
	d := testDB(t)
	s := newTestService(d)

	const users = 40
	for i := 0; i < users; i++ {
		uid := mkUser(t, d, fmt.Sprintf("u%d", i), "es")
		if err := d.UpsertPushSubscription(fmt.Sprintf("s%d", i), uid, fmt.Sprintf("https://push.example/%d", i), "p256", "auth", "ua", time.Now().UnixMilli()); err != nil {
			t.Fatalf("sub: %v", err)
		}
	}

	var inFlight, maxInFlight atomic.Int32
	s.SetSendFn(func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
		cur := inFlight.Add(1)
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		return 201, nil
	})

	ids, err := d.AllUserIDs()
	if err != nil {
		t.Fatalf("ids: %v", err)
	}
	res := s.NotifyUsers(ids, "bateria_baja", Datos{"soc": 5.0}, Options{})

	if res.Enviados != users {
		t.Fatalf("enviados = %d, esperado %d", res.Enviados, users)
	}
	if maxInFlight.Load() > maxConcurrentSends {
		t.Fatalf("envíos simultáneos = %d, máximo permitido %d", maxInFlight.Load(), maxConcurrentSends)
	}
}

type optionsGuard struct {
	mu   sync.Mutex
	seen []*webpush.Options
}

func newOptionsGuard(t *testing.T) *optionsGuard {
	t.Helper()
	return &optionsGuard{}
}

func (g *optionsGuard) fn(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
	g.mu.Lock()
	g.seen = append(g.seen, opts)
	g.mu.Unlock()
	return 201, nil
}

func (g *optionsGuard) assert(t *testing.T) {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.seen) == 0 {
		t.Fatal("no se intentó ningún envío")
	}
	for _, opts := range g.seen {
		if opts.HTTPClient == nil {
			t.Fatal("el envío usa el cliente HTTP por defecto, sin timeout")
		}
		client, ok := opts.HTTPClient.(*http.Client)
		if !ok {
			continue
		}
		if client.Timeout <= 0 {
			t.Fatalf("timeout = %v, esperado mayor que cero", client.Timeout)
		}
		if client.CheckRedirect == nil {
			t.Fatal("el cliente HTTP sigue redirects")
		}
	}
}
