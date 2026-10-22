package push

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"helios-server/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "push.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func mkUser(t *testing.T, d *db.DB, username, lang string) string {
	t.Helper()
	u, err := d.CreateUser(username, "hash", lang, "user")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u.ID
}

func newTestService(d *db.DB) *Service {
	return New(d, "pub", "priv", "mailto:test@example.com")
}

func TestNotConfiguredOmits(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := New(d, "", "", "")

	if s.IsConfigured() {
		t.Fatal("no debería estar configurado")
	}

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{"soc": 10.0}, Options{})
	if res.Omitidos == 0 {
		t.Fatalf("debería omitir sin VAPID: %+v", res)
	}
}

func TestNotifySendsToSubscriptions(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)

	if err := d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p256", "auth", "ua", time.Now().UnixMilli()); err != nil {
		t.Fatalf("sub: %v", err)
	}

	var gotPayload string
	s.SetSendFn(func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
		gotPayload = string(payload)
		if opts.TTL <= 0 {
			t.Errorf("TTL inválido: %d", opts.TTL)
		}
		return 201, nil
	})

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{"soc": 12.0, "reserva": 20.0}, Options{Severity: "high"})
	if res.Enviados != 1 {
		t.Fatalf("enviados = %d (%+v)", res.Enviados, res)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(gotPayload), &payload); err != nil {
		t.Fatalf("payload no es JSON: %v", err)
	}
	if payload["title"] != "Batería baja" {
		t.Fatalf("title = %v", payload["title"])
	}
	if !strings.Contains(payload["body"].(string), "12%") {
		t.Fatalf("body = %v", payload["body"])
	}
	if payload["tag"] != "bateria_baja" {
		t.Fatalf("tag = %v", payload["tag"])
	}
}

func TestNotifyRespectsDisabledPreference(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())
	d.SetPreference(uid, "bateria_baja", false, time.Now().UnixMilli())

	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) {
		t.Fatal("no debería enviar con preferencia desactivada")
		return 0, nil
	})

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{}, Options{})
	if res.Omitidos != 1 {
		t.Fatalf("omitidos = %d", res.Omitidos)
	}
}

func TestNotifyRespectsMinSeverity(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	// min_severity = critical
	if _, err := d.Exec(
		`INSERT INTO notification_preferences (user_id, tipo, enabled, min_severity, updated_at)
		 VALUES (?, 'bateria_baja', 1, 'critical', ?)`, uid, time.Now().UnixMilli(),
	); err != nil {
		t.Fatalf("pref: %v", err)
	}

	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) {
		t.Fatal("severidad high no debería pasar con min_severity critical")
		return 0, nil
	})

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{}, Options{Severity: "high"})
	if res.Omitidos != 1 {
		t.Fatalf("omitidos = %d", res.Omitidos)
	}
}

func TestNotifyQuietHoursEnqueues(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	// Ventana que cubre AHORA en Europe/Madrid.
	loc, _ := time.LoadLocation("Europe/Madrid")
	h := time.Now().In(loc).Hour()
	start := h
	end := (h + 2) % 24
	if _, err := d.Exec(
		`INSERT INTO notification_quiet_hours (user_id, quiet_start, quiet_end, tz, updated_at)
		 VALUES (?, ?, ?, 'Europe/Madrid', ?)`, uid, start, end, time.Now().UnixMilli(),
	); err != nil {
		t.Fatalf("quiet: %v", err)
	}

	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) {
		t.Fatal("no debería enviar en horas de silencio")
		return 0, nil
	})

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{"soc": 10.0}, Options{Severity: "normal"})
	if res.Pospuestos != 1 {
		t.Fatalf("pospuestos = %d (%+v)", res.Pospuestos, res)
	}

	groups, err := d.QueueGroups()
	if err != nil || len(groups) != 1 {
		t.Fatalf("cola = %v (err %v)", groups, err)
	}
}

func TestCriticalBypassesQuietHours(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	loc, _ := time.LoadLocation("Europe/Madrid")
	h := time.Now().In(loc).Hour()
	d.Exec(`INSERT INTO notification_quiet_hours (user_id, quiet_start, quiet_end, tz, updated_at)
	        VALUES (?, ?, ?, 'Europe/Madrid', ?)`, uid, h, (h+2)%24, time.Now().UnixMilli())

	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) { return 201, nil })

	res := s.NotifyUsers([]string{uid}, "corte_red", Datos{}, Options{Severity: "critical"})
	if res.Enviados != 1 {
		t.Fatalf("critical debería enviarse: %+v", res)
	}
}

func TestDeadSubscriptionRemoved(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) { return 410, nil })

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{}, Options{})
	if res.Borrados != 1 {
		t.Fatalf("borrados = %d (%+v)", res.Borrados, res)
	}
	subs, _ := d.SubscriptionsByUser(uid)
	if len(subs) != 0 {
		t.Fatalf("suscripción no borrada: %v", subs)
	}
}

func TestRetriesOn500(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	attempts := 0
	s.SetSendFn(func(*webpush.Subscription, []byte, *webpush.Options) (int, error) {
		attempts++
		if attempts < 3 {
			return 500, nil
		}
		return 201, nil
	})

	res := s.NotifyUsers([]string{uid}, "bateria_baja", Datos{}, Options{})
	if res.Enviados != 1 {
		t.Fatalf("debería acabar enviando: %+v", res)
	}
	if attempts != 3 {
		t.Fatalf("intentos = %d (esperado 3)", attempts)
	}
}

func TestPayloadI18n(t *testing.T) {
	cases := []struct {
		lang  Lang
		tipo  string
		title string
	}{
		{LangES, "bateria_llena", "Batería llena"},
		{LangEN, "bateria_llena", "Battery full"},
		{LangZH, "bateria_llena", "电池已满"},
	}
	for _, c := range cases {
		raw := composePayload(c.lang, c.tipo, Datos{"soc": 100.0}, "/")
		var p map[string]any
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("json: %v", err)
		}
		if p["title"] != c.title {
			t.Fatalf("%s title = %v (esperado %v)", c.lang, p["title"], c.title)
		}
	}
}

func TestPayloadUnknownTipoFallsBack(t *testing.T) {
	raw := composePayload(LangES, "desconocido", Datos{"total": 5.0}, "/")
	var p map[string]any
	json.Unmarshal(raw, &p)
	if p["title"] != "Actividad en Helios" {
		t.Fatalf("fallback title = %v", p["title"])
	}
}

func TestFlushNotificationQueue(t *testing.T) {
	d := testDB(t)
	uid := mkUser(t, d, "u1", "es")
	s := newTestService(d)
	d.UpsertPushSubscription("s1", uid, "https://push.example/1", "p", "a", "ua", time.Now().UnixMilli())

	// Cola con 3 alertas (sin quiet hours activas).
	for i := 0; i < 3; i++ {
		d.EnqueueNotification(newUUID(), uid, "bateria_baja", "normal", `{"soc":10}`, time.Now().UnixMilli())
	}

	var sent []string
	s.SetSendFn(func(sub *webpush.Subscription, payload []byte, opts *webpush.Options) (int, error) {
		sent = append(sent, string(payload))
		return 201, nil
	})

	s.FlushNotificationQueue()

	if len(sent) != 1 {
		t.Fatalf("debería enviar 1 resumen consolidado, envió %d", len(sent))
	}
	if !strings.Contains(sent[0], "3 alertas") {
		t.Fatalf("payload = %s", sent[0])
	}
	groups, _ := d.QueueGroups()
	if len(groups) != 0 {
		t.Fatalf("cola no vaciada: %v", groups)
	}
}
