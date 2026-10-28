package main

import (
	"testing"

	"helios-server/internal/config"
)

func TestNewHAClientRejectsCleartextRemoteTarget(t *testing.T) {
	cfg := &config.Config{HAOSURL: "http://192.168.10.244:8123", HAOSToken: "tok"}
	if _, err := newHAClient(cfg); err == nil {
		t.Fatal("no se debe crear un cliente HA sobre HTTP remoto")
	}
}

func TestNewHAClientAcceptsTLSAndExplicitOptIn(t *testing.T) {
	for _, cfg := range []*config.Config{
		{HAOSURL: "https://192.168.10.244:8123", HAOSToken: "tok"},
		{HAOSURL: "http://127.0.0.1:8123", HAOSToken: "tok"},
		{HAOSURL: "http://192.168.10.244:8123", HAOSToken: "tok", HAOSAllowInsecure: true},
	} {
		client, err := newHAClient(cfg)
		if err != nil {
			t.Fatalf("%s: %v", cfg.HAOSURL, err)
		}
		if client == nil {
			t.Fatalf("%s: cliente nil", cfg.HAOSURL)
		}
	}
}

func TestNewHAClientRejectsMissingToken(t *testing.T) {
	client, err := newHAClient(&config.Config{HAOSURL: "https://ha.example.com"})
	if err != nil || client != nil {
		t.Fatalf("sin token no debe crearse cliente: client=%v err=%v", client, err)
	}
}
