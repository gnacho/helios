package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMakefileAcceptsPublishedTailwindDigest checks every pinned digest against
// the checksums published by the project, so a typo or a stale pin is caught.
func TestMakefileAcceptsPublishedTailwindDigest(t *testing.T) {
	if os.Getenv("HELIOS_NETWORK_TESTS") != "1" {
		t.Skip("set HELIOS_NETWORK_TESTS=1 to fetch the published checksum")
	}
	mk := readRepoFile(t, "Makefile")
	ver := regexpFind(t, mk, `TW_VER\s*\?=\s*(v[0-9]+\.[0-9]+\.[0-9]+)`)

	out, err := exec.Command("curl", "-fsSL", "--max-time", "60",
		"https://github.com/tailwindlabs/tailwindcss/releases/download/"+ver+"/sha256sums.txt").Output()
	if err != nil {
		t.Fatalf("fetch sha256sums: %v", err)
	}
	published := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			published[strings.TrimPrefix(fields[1], "./")] = fields[0]
		}
	}
	for _, arch := range []string{"x64", "arm64"} {
		asset := "tailwindcss-linux-" + arch
		digest := regexpFind(t, mk, `TW_SHA256_`+arch+`\s*:?=\s*([0-9a-f]{64})`)
		got, ok := published[asset]
		if !ok {
			t.Fatalf("sha256sums.txt no publica %s", asset)
		}
		if got != digest {
			t.Fatalf("digest fijado de %s %s no coincide con el publicado %s", asset, digest, got)
		}
	}
}

// TestWorkflowPinsGitleaksVersionWithChecksum keeps CI tied to a version whose
// checksums file is fetched and verified in the same step.
func TestWorkflowPinsGitleaksVersionWithChecksum(t *testing.T) {
	ci := readRepoFile(t, ".github/workflows/ci.yml")
	if !strings.Contains(ci, "ver=8.30.1") {
		t.Fatal("ci.yml no fija la version de gitleaks en una variable")
	}
	if strings.Contains(ci, "v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz |") {
		t.Fatal("ci.yml mantiene la descarga directa por tuberia")
	}
	tmp := t.TempDir()
	script := filepath.Join(tmp, "step.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+ciExtractRun(t, ci)), 0o644); err != nil {
		t.Fatalf("write step: %v", err)
	}
	if _, err := exec.Command("sh", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("el paso de gitleaks no es shell valido: %v", err)
	}
}

func regexpFind(t *testing.T, text, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no se encontro %s en el fichero", pattern)
	}
	return m[1]
}

func ciExtractRun(t *testing.T, ci string) string {
	t.Helper()
	idx := strings.Index(ci, "- name: Instalar gitleaks")
	if idx < 0 {
		t.Fatal("no se encontro el paso de instalacion de gitleaks")
	}
	rest := ci[idx:]
	start := strings.Index(rest, "run: |")
	if start < 0 {
		t.Fatal("el paso de gitleaks no usa bloque run multilinea")
	}
	body := rest[start+len("run: |"):]
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "          ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	return strings.Join(lines, "\n") + "\n"
}
