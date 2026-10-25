#!/bin/sh
# helios-update.sh — aplica la última release ESTABLE de Helios (runtime Go).
#
# Layout en el CT:
#   /opt/helios/
#     server/helios-go   → BINARIO (se sustituye en cada update)
#     server/.env        → configuración (NUNCA se toca)
#     public/            → OBSOLETO: los estáticos (favicon, manifest, sw.js)
#                          viajan dentro del binario desde la retirada del SPA
#     data/              → SQLite, avatares, imágenes (NUNCA se toca)
#     .release-id        → versión instalada (fuente de verdad de /api/update/status)
#
# Assets del release:
#   helios_<ver>_linux_<arch>  → binario estático (única pieza: la UI y los
#                                estáticos van embebidos)
#   checksums.txt              → sha256 del binario
#
# Garantías:
#   - descarga a tmp, verifica sha256 ANTES de instalar
#   - backup del binario anterior
#   - sustitución atómica del binario (mv sobre el mismo filesystem)
#   - si el servicio no arranca tras el restart, ROLLBACK automático al binario
#     anterior y exit != 0
set -eu

APP=helios
REPO=gnacho/helios
ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
OPT_DIR=/opt/helios
STATE_DIR=/opt/helios/data
MARKER="$OPT_DIR/.release-id"
ENV_FILE="$OPT_DIR/server/.env"
BIN="$OPT_DIR/server/helios-go"
SERVICE_NAME=helios-go
PROGRESS_FILE="$STATE_DIR/update-progress.json"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

log() { logger -t "$APP-update" "$@"; }

progress() {
  printf '{"step":"%s","pct":%d,"ts":%d}\n' "$1" "$2" "$(date +%s)" > "$PROGRESS_FILE"
  echo "STEP:$1"
}

rm -f "$STATE_DIR/.update-requested" 2>/dev/null || true

progress "fetch" 5
VER="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\(v\?[0-9][^"]*\)".*/\1/p' | head -n1)"
[ -n "$VER" ] || { log "no se pudo resolver release latest"; progress "error" 0; exit 4; }
VER_NO_V="$(printf '%s' "$VER" | sed 's/^v//')"

if [ -f "$MARKER" ] && [ "$(cat "$MARKER" 2>/dev/null || true)" = "$VER_NO_V" ]; then
  log "al día ($VER_NO_V)"; rm -f "$PROGRESS_FILE"; exit 0
fi

progress "download" 15
BIN_NAME="helios_${VER_NO_V}_linux_${ARCH}"
BASE="https://github.com/$REPO/releases/download/$VER"
curl -fL "$BASE/$BIN_NAME" -o "$TMP_DIR/helios-go"
progress "download" 55
TS="$(date +%s)"
curl -fL "$BASE/checksums.txt?nc=$TS" -o "$TMP_DIR/checksums.txt"

progress "verify" 60
EXPECTED_BIN="$(awk -v f="$BIN_NAME" '$0 ~ f {print $1; exit}' "$TMP_DIR/checksums.txt")"
[ -n "$EXPECTED_BIN" ] || { log "checksums.txt sin entrada para $BIN_NAME"; progress "error" 0; exit 5; }
GOT_BIN="$(sha256sum "$TMP_DIR/helios-go" | awk '{print $1}')"
[ "$EXPECTED_BIN" = "$GOT_BIN" ] || { log "SHA256 binario NO coincide"; progress "error" 0; exit 5; }
progress "verify" 70

progress "install" 72
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP_BIN="$OPT_DIR/server/helios-go.bak-$TS"
[ -f "$BIN" ] && cp -a "$BIN" "$BACKUP_BIN"

# Binario: copia al lado y mv atómico (el servicio está parado durante el swap)
install -m 0755 "$TMP_DIR/helios-go" "$OPT_DIR/server/helios-go.new"
chown "$APP:$APP" "$OPT_DIR/server/helios-go.new" 2>/dev/null || true

progress "install" 88
mv -f "$OPT_DIR/server/helios-go.new" "$BIN"
chown "$APP:$APP" "$BIN" 2>/dev/null || true

progress "restart" 92
printf '%s' "$VER_NO_V" > "$MARKER"
chmod 0644 "$MARKER"

if [ "${SKIP_RESTART:-0}" != "1" ]; then
  systemctl restart "$SERVICE_NAME"
  # Espera a que arranque y responde; si no, rollback.
  OK=0
  i=0
  while [ $i -lt 15 ]; do
    sleep 1
    if curl -fsS --max-time 3 http://127.0.0.1/health >/dev/null 2>&1; then OK=1; break; fi
    i=$((i+1))
  done
  if [ "$OK" != "1" ]; then
    log "el servicio no respondió tras actualizar: ROLLBACK a la versión anterior"
    if [ -f "$BACKUP_BIN" ]; then
      mv -f "$BACKUP_BIN" "$BIN"
      systemctl restart "$SERVICE_NAME" || true
    fi
    progress "error" 0
    exit 6
  fi
fi

log "actualizado a $VER_NO_V"
progress "done" 100
