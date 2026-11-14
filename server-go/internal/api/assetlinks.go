package api

import (
	"encoding/json"
	"net/http"
)

// Digital Asset Links for the Android app.
//
// Chrome only hides the address bar in a Trusted Web Activity when the site
// publishes this file and the certificate matches the app that is requesting
// it. The fingerprint below is the SHA-256 of the release keystore
// (helios-release.jks, alias "helios"): if that keystore is ever replaced, this
// constant must change too, or every installed app falls back to a plain tab.
//
// The route lives in the binary on purpose. It has to answer on every origin an
// installed app may point at, and the app is pointed at the instance, not at the
// reverse proxy.
const (
	androidPackageName = "club.cloudless.helios"

	androidCertSHA256 = "D7:41:06:B7:84:79:9F:AD:42:0D:63:B0:ED:E1:D9:BC:1D:47:08:3F:20:DA:95:AE:6A:9B:E8:17:3B:70:B2:A1"
)

type assetLinkTarget struct {
	Namespace             string   `json:"namespace"`
	PackageName           string   `json:"package_name"`
	SHA256CertFingerprint []string `json:"sha256_cert_fingerprints"`
}

type assetLink struct {
	Relation []string        `json:"relation"`
	Target   assetLinkTarget `json:"target"`
}

// handleAssetLinks serves /.well-known/assetlinks.json. Public and cacheable:
// the file only changes when the signing key does.
func (s *Server) handleAssetLinks(w http.ResponseWriter, r *http.Request) {
	payload := []assetLink{{
		Relation: []string{"delegate_permission/common.handle_all_urls"},
		Target: assetLinkTarget{
			Namespace:             "android_app",
			PackageName:           androidPackageName,
			SHA256CertFingerprint: []string{androidCertSHA256},
		},
	}}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}
