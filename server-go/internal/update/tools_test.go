package update

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../../.."

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(repoRoot + "/" + rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// TestMakefileVerifiesTailwindChecksum requires the standalone binary to be
// verified against the published sha256sums.txt before it is executed.
func TestMakefileVerifiesTailwindChecksum(t *testing.T) {
	mk := readRepoFile(t, "Makefile")
	if !strings.Contains(mk, "sha256sums.txt") {
		t.Fatal("el Makefile descarga Tailwind sin comprobar sha256sums.txt")
	}
	if !strings.Contains(mk, "sha256sum -c") {
		t.Fatal("el Makefile no verifica el hash de Tailwind con sha256sum -c")
	}
	if strings.Contains(mk, "curl -sL -o $(TW)") && !strings.Contains(mk, "sha256sum -c") {
		t.Fatal("la descarga de Tailwind sigue siendo directa a destino final")
	}
}

// TestMakefilePinsTailwindDigest requires a concrete digest for the pinned
// version so a compromised release asset cannot swap it silently.
func TestMakefilePinsTailwindDigest(t *testing.T) {
	mk := readRepoFile(t, "Makefile")
	if !regexp.MustCompile(`TW_SHA256\s*\?=\s*[0-9a-f]{64}`).MatchString(mk) {
		t.Fatal("falta TW_SHA256 con el digest fijado de Tailwind")
	}
	if !regexp.MustCompile(`TW_VER\s*\?=\s*v[0-9]+\.[0-9]+\.[0-9]+`).MatchString(mk) {
		t.Fatal("falta TW_VER fijada junto al digest")
	}
	if !regexp.MustCompile(`TW_ASSET\s*\?=\s*tailwindcss-linux-x64`).MatchString(mk) {
		t.Fatal("falta TW_ASSET que fija el artefacto descargado")
	}
}

// TestWorkflowVerifiesGitleaksChecksum requires the CI download of gitleaks to
// be checked against the release checksums file.
func TestWorkflowVerifiesGitleaksChecksum(t *testing.T) {
	ci := readRepoFile(t, ".github/workflows/ci.yml")
	if !strings.Contains(ci, "checksums.txt") {
		t.Fatal("ci.yml descarga gitleaks sin el fichero de checksums de la release")
	}
	if !strings.Contains(ci, "sha256sum -c") {
		t.Fatal("ci.yml no verifica el tarball de gitleaks con sha256sum -c")
	}
	if strings.Contains(ci, "tar.gz | sudo tar xz") {
		t.Fatal("ci.yml sigue extrayendo gitleaks directamente desde la red")
	}
	if !sha256Hex.MatchString("551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb") {
		t.Fatal("sanidad del patron sha256")
	}
}
