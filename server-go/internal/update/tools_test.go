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
// version so a compromised release asset cannot swap it silently, and one
// digest per architecture so the release matrix can run the binary it
// downloads.
func TestMakefilePinsTailwindDigest(t *testing.T) {
	mk := readRepoFile(t, "Makefile")
	for _, arch := range []string{"x64", "arm64"} {
		re := regexp.MustCompile(`TW_SHA256_` + arch + `\s*:?=\s*[0-9a-f]{64}`)
		if !re.MatchString(mk) {
			t.Fatalf("falta TW_SHA256_%s con el digest fijado de Tailwind", arch)
		}
	}
	if !regexp.MustCompile(`TW_VER\s*\?=\s*v[0-9]+\.[0-9]+\.[0-9]+`).MatchString(mk) {
		t.Fatal("falta TW_VER fijada junto al digest")
	}
	if !regexp.MustCompile(`TW_ASSET\s*\?=\s*tailwindcss-linux-\$\(TW_ARCH\)`).MatchString(mk) {
		t.Fatal("TW_ASSET debe elegir el artefacto segun la arquitectura del runner")
	}
	if !strings.Contains(mk, "TW_SHA256 ?= $(TW_SHA256_$(TW_ARCH))") {
		t.Fatal("TW_SHA256 debe resolver el digest de la arquitectura elegida")
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
