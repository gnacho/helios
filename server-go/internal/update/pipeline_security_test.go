//go:build linux

package update

import (
	"regexp"
	"strings"
	"testing"
)

func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	return readRepoFile(t, strings.Join(parts, "/"))
}

func TestReleaseWorkflowDoesNotInterpolateRefInShell(t *testing.T) {
	script := readFile(t, ".github", "workflows", "release.yml")
	if !strings.Contains(script, "GITHUB_REF_NAME: ${{ github.ref_name }}") {
		t.Fatal("release.yml no pasa el tag por entorno GITHUB_REF_NAME")
	}
	if !strings.Contains(script, "GITHUB_REF_TYPE: ${{ github.ref_type }}") {
		t.Fatal("release.yml no pasa el tipo de ref por entorno GITHUB_REF_TYPE")
	}
	// Every remaining `${{ github.ref... }}` must live in an env: or if: line,
	// never inside a run: block where it would be interpolated into shell.
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "${{ github.ref") {
			continue
		}
		if strings.HasPrefix(trimmed, "if:") {
			continue
		}
		if strings.HasPrefix(trimmed, "GITHUB_REF_NAME:") || strings.HasPrefix(trimmed, "GITHUB_REF_TYPE:") {
			continue
		}
		t.Fatalf("release.yml interpola %q fuera de env/if", trimmed)
	}
}

func TestReleaseWorkflowValidatesTagFormat(t *testing.T) {
	script := readFile(t, ".github", "workflows", "release.yml")
	tagRe := regexp.MustCompile(`\^v\[0-9\]\+\\\.\[0-9\]\+\\\.\[0-9\]\+`)
	if !tagRe.MatchString(script) {
		t.Fatal("release.yml no valida el formato vX.Y.Z del tag")
	}
	if !strings.Contains(script, "grep -Eq") && !strings.Contains(script, "grep -qE") {
		t.Fatal("release.yml no valida el tag con grep -E")
	}
}

func TestReleaseBuildCheckoutDoesNotPersistCredentials(t *testing.T) {
	script := readFile(t, ".github", "workflows", "release.yml")
	build := script[strings.Index(script, "\n  build:"):strings.Index(script, "\n  release:")]
	if !strings.Contains(build, "persist-credentials: false") {
		t.Fatal("el job build no desactiva persist-credentials en el checkout")
	}
}

func TestReleaseWorkflowRunsWebCheckBeforeBuild(t *testing.T) {
	script := readFile(t, ".github", "workflows", "release.yml")
	if !strings.Contains(script, "make web-check") {
		t.Fatal("release.yml no repite la comprobación de artefactos web")
	}
	webCheck := strings.Index(script, "make web-check")
	goBuild := strings.Index(script, "go build -trimpath")
	if webCheck < 0 || goBuild < 0 || webCheck > goBuild {
		t.Fatal("la comprobación de artefactos web no precede a la compilación")
	}
}

func TestMakefileWebCheckDetectsUntrackedArtifacts(t *testing.T) {
	script := readFile(t, "Makefile")
	start := strings.Index(script, "web-check:")
	if start < 0 {
		t.Fatal("Makefile sin objetivo web-check")
	}
	block := script[start:]
	if end := strings.Index(block, "\n\n"); end > 0 {
		block = block[:end]
	}
	if !strings.Contains(block, "git ls-files --others --exclude-standard") {
		t.Fatal("web-check no detecta archivos generados sin rastrear")
	}
	if !strings.Contains(block, "git diff --exit-code") {
		t.Fatal("web-check dejó de comprobar los archivos rastreados")
	}
}

func TestLandingWorkflowChecksBuildArtifacts(t *testing.T) {
	script := readFile(t, ".github", "workflows", "ci.yml")
	landing := script[strings.Index(script, "\n  landing:"):strings.Index(script, "\n  gitleaks:")]
	if !strings.Contains(landing, "git status --porcelain") {
		t.Fatal("el job landing no comprueba que el build no deje cambios sin commitear")
	}
}
