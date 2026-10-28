package ha

import (
	"net/url"
	"strings"
	"testing"
)

func TestValidateHAURLRequiresTLSOutsideLoopback(t *testing.T) {
	rejected := []string{
		"http://192.168.10.244:8123",
		"http://homeassistant.local:8123",
		"ws://192.168.10.244:8123",
		"ftp://192.168.10.244:8123",
		"http://user:pass@192.168.10.244:8123",
	}
	for _, raw := range rejected {
		if err := ValidateHAURL(raw, false); err == nil {
			t.Fatalf("%s debería rechazarse sin excepción explícita", raw)
		}
	}

	accepted := []string{
		"https://192.168.10.244:8123",
		"wss://192.168.10.244:8123",
		"https://ha.example.com",
		"http://127.0.0.1:8123",
		"http://[::1]:8123",
		"ws://localhost:8123",
	}
	for _, raw := range accepted {
		if err := ValidateHAURL(raw, false); err != nil {
			t.Fatalf("%s debería aceptarse: %v", raw, err)
		}
	}
}

func TestValidateHAURLAllowsExplicitInsecureOptIn(t *testing.T) {
	if err := ValidateHAURL("http://192.168.10.244:8123", true); err != nil {
		t.Fatalf("la excepción explícita debería permitir HTTP en red local: %v", err)
	}
	if err := ValidateHAURL("ftp://192.168.10.244:8123", true); err == nil {
		t.Fatal("un esquema no soportado debe rechazarse aunque haya opt-in explícito")
	}
}

func TestValidateHAURLRejectsMalformedValues(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a url", "http://", "https:///path"} {
		if err := ValidateHAURL(raw, true); err == nil {
			t.Fatalf("%q debería rechazarse", raw)
		}
	}
}

func TestNewClientNormalizesWebsocketScheme(t *testing.T) {
	cases := map[string]string{
		"https://ha.example.com:8123": "wss://ha.example.com:8123/api/websocket",
		"http://127.0.0.1:8123":       "ws://127.0.0.1:8123/api/websocket",
		"wss://ha.example.com/":       "wss://ha.example.com/api/websocket",
		"http://192.168.10.244:8123/": "ws://192.168.10.244:8123/api/websocket",
	}
	for in, want := range cases {
		c := NewClient(in, "tok")
		if c.wsURL != want {
			t.Fatalf("%s: wsURL = %q, esperado %q", in, c.wsURL, want)
		}
	}
}

func TestHistoryURLKeepsEncodedQueryParameters(t *testing.T) {
	c := NewClient("https://ha.example.com:8123", "tok")
	u, err := url.Parse(c.httpURL + "/api/history/period/x")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q, esperado https", u.Scheme)
	}
	if strings.HasSuffix(c.httpURL, "/") {
		t.Fatalf("httpURL no debería terminar en barra: %q", c.httpURL)
	}
}
