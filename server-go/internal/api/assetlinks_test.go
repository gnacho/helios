package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetLinksIsPublicAndWellFormed(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil)

	s.handleAssetLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age") {
		t.Fatalf("cache control = %q", cc)
	}

	var payload []struct {
		Relation []string `json:"relation"`
		Target   struct {
			Namespace    string   `json:"namespace"`
			PackageName  string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		} `json:"target"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if len(payload) != 1 {
		t.Fatalf("statements = %d, want 1", len(payload))
	}
	if got := payload[0].Target.PackageName; got != "club.cloudless.helios" {
		t.Fatalf("package = %q", got)
	}
	if payload[0].Target.Namespace != "android_app" {
		t.Fatalf("namespace = %q", payload[0].Target.Namespace)
	}
	if len(payload[0].Relation) != 1 || payload[0].Relation[0] != "delegate_permission/common.handle_all_urls" {
		t.Fatalf("relation = %v", payload[0].Relation)
	}
	if len(payload[0].Target.Fingerprints) != 1 {
		t.Fatalf("fingerprints = %v", payload[0].Target.Fingerprints)
	}
	if !strings.HasPrefix(payload[0].Target.Fingerprints[0], "D7:41:06:B7") {
		t.Fatalf("fingerprint does not look like the release keystore: %q", payload[0].Target.Fingerprints[0])
	}
}
