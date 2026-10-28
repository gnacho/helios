//go:build linux

package update

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func scriptPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../../deploy/helios-update.sh")
	if err != nil {
		t.Fatalf("abs script: %v", err)
	}
	return path
}

func readScript(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("cat", scriptPath(t)).Output()
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	return string(out)
}

func TestDeployScriptSyntax(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sh disponible solo en linux")
	}
	cmd := exec.Command("sh", "-n", scriptPath(t))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n falló: %v\n%s", err, out)
	}
}

// El rollback solo restaura el binario hoy. Si el marker ya se escribió con la
// versión nueva, el estado queda mintiendo: la UI enseña una versión que no
// corre. Exigimos que el marker anterior se guarde y se restaure.
func TestDeployScriptRollsBackMarker(t *testing.T) {
	src := readScript(t)
	if !strings.Contains(src, "BACKUP_MARKER") {
		t.Fatal("el script no guarda copia del marker anterior para poder restaurarlo")
	}
	rollback := src[strings.Index(src, "ROLLBACK"):]
	if !strings.Contains(rollback, "BACKUP_MARKER") {
		t.Fatal("el rollback no restaura el marker de la versión anterior")
	}
}

// El marker se escribe antes del restart, así que un arranque fallido publica una
// versión que no está corriendo. Debe escribirse solo cuando el health check ya
// ha pasado, es decir, después del bloque que espera el arranque.
func TestDeployScriptWritesMarkerAfterHealthCheck(t *testing.T) {
	src := readScript(t)
	markerWrite := strings.Index(src, `printf '%s' "$VER_NO_V" > "$MARKER"`)
	if markerWrite < 0 {
		t.Fatal("no se encontró la escritura del marker")
	}
	healthLoop := strings.Index(src, "while [ $i -lt 15 ]")
	if healthLoop < 0 {
		t.Fatal("no se encontró el bucle de health check")
	}
	if markerWrite < healthLoop {
		t.Fatal("el marker se escribe antes de esperar el health check")
	}
}

// El script solo comparaba desigualdad: una release antigua publicada como latest
// reinstalaba una versión menor encima de la actual.
func TestDeployScriptRejectsDowngrade(t *testing.T) {
	src := readScript(t)
	if !strings.Contains(src, "ver_is_newer") {
		t.Fatal("el script no compara versiones: cualquier release distinta se instala, incluido un downgrade")
	}
}

// El health check apuntaba al puerto 80 fijo aunque el servicio escuche otro.
func TestDeployScriptDerivesHealthPort(t *testing.T) {
	src := readScript(t)
	if strings.Contains(src, "http://127.0.0.1/health") {
		t.Fatal("el health check usa un puerto fijo y falla cuando PORT no es 80")
	}
	if !strings.Contains(src, "HELIOS_HEALTH_PORT") && !strings.Contains(src, "PORT") {
		t.Fatal("el health check no deriva el puerto configurado")
	}
}

// curl sin límites puede colgar el updater indefinidamente (lo ejecuta systemd
// como root con TimeoutStartSec).
func TestDeployScriptLimitsCurl(t *testing.T) {
	src := readScript(t)
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "curl ") {
			continue
		}
		if !strings.Contains(trimmed, "--max-time") {
			t.Fatalf("curl sin --max-time: %s", trimmed)
		}
	}
}

func TestDeployScriptWritesProgressWithoutFollowingSymlinks(t *testing.T) {
	s := readScript(t)
	if strings.Contains(s, "> \"$PROGRESS_FILE\"") {
		t.Fatal("el progreso se escribe con redirección directa, que sigue symlinks")
	}
	if !strings.Contains(s, "ln -T") && !strings.Contains(s, "noclobber") && !strings.Contains(s, "mktemp") {
		t.Fatal("el progreso no usa escritura segura ni temporal")
	}
	if !strings.Contains(s, "chmod 0644") && !strings.Contains(s, "umask") {
		t.Fatal("el progreso no fija permisos al crearlo")
	}
}

func TestDeployScriptProgressFileIsRootOwned(t *testing.T) {
	s := readScript(t)
	if !strings.Contains(s, "PROGRESS_DIR") {
		t.Fatal("no hay directorio separado y root-owned para el progreso")
	}
	if strings.Contains(s, "PROGRESS_FILE=\"$STATE_DIR/update-progress.json\"") {
		t.Fatal("el progreso sigue dentro del directorio escribible por el servicio")
	}
}

func TestDeployScriptPublishesProgressWhereServerReadsIt(t *testing.T) {
	s := readScript(t)
	if !strings.Contains(s, `ln -sfn`) {
		t.Fatal("el script no publica el progreso en la ruta que lee el servidor")
	}
	if !strings.Contains(s, `"$STATE_DIR/update-progress.json"`) {
		t.Fatal("el enlace publicado no apunta a la ruta que lee el servidor")
	}
}

func TestDeployScriptInstallsRootOwnedBinary(t *testing.T) {
	s := readScript(t)
	if !strings.Contains(s, `ROOT_OWNED_MODE`) {
		t.Fatal("no se fija el modo del binario propiedad de root")
	}
	if strings.Contains(s, `chown "$APP:$APP" "$BIN"`) {
		t.Fatal("el binario instalado queda propiedad del usuario del servicio")
	}
	if !strings.Contains(s, `chmod "$ROOT_OWNED_MODE" "$BIN"`) {
		t.Fatal("el binario instalado no fija su modo tras el swap")
	}
	if !strings.Contains(s, `AUDIT_GROUP`) && !strings.Contains(s, `chmod 0750 "$OPT_DIR/server"`) {
		t.Fatal("el directorio del binario no restringe la escritura al servicio")
	}
}

func TestDeployScriptKeepsBackupsRootOnly(t *testing.T) {
	s := readScript(t)
	if !strings.Contains(s, `chmod "$BACKUP_MODE" "$BACKUP_BIN"`) {
		t.Fatal("el backup del binario no restringe permisos")
	}
	if !strings.Contains(s, `chmod 0600 "$BACKUP_MARKER"`) {
		t.Fatal("el backup del marker no restringe permisos")
	}
}

func TestDeployScriptRestrictsRedirectsAndTransport(t *testing.T) {
	s := readScript(t)
	if !strings.Contains(s, `CURL_NET=`) {
		t.Fatal("no se define una política de transporte común para curl")
	}
	if !strings.Contains(s, `--proto =https`) || !strings.Contains(s, `--proto-redir =https`) {
		t.Fatal("la política de curl no fija HTTPS para la petición y los redirects")
	}
	if !strings.Contains(s, `--max-redirs`) {
		t.Fatal("la política de curl no limita los redirects")
	}
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "curl ") {
			continue
		}
		if !strings.Contains(trimmed, "$CURL_NET") && !strings.Contains(trimmed, "127.0.0.1") {
			t.Fatalf("curl sin politica de transporte: %s", trimmed)
		}
	}
}
