#!/bin/sh
# helios-update.sh — aplica la última release ESTABLE de Helios.
# Patrón app-auto-update (variante Node), layout PLANO del CT:
#   /opt/helios/{server,public,shared}
#     server/  → código (src/) + package.json + node_modules + .env (se conserva)
#     public/  → dist/ del frontend
#     shared/  → schemas compartidos
#   datos en $STATE_DIR (/opt/helios/data, o server/data) — NUNCA se tocan.
# El marker /opt/helios/.release-id es la fuente de verdad de la versión
# instalada (server/src/update.js la lee para /api/update/status).
set -eu

APP=helios
REPO=gnacho/helios
ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
OPT_DIR=/opt/helios
STATE_DIR=/opt/helios/data
MARKER="$OPT_DIR/.release-id"
ENV_FILE="$OPT_DIR/server/.env"
SERVICE_NAME=helios
PROGRESS_FILE="$STATE_DIR/update-progress.json"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

log() { logger -t "$APP-update" "$@"; }

progress() {
  printf '{"step":"%s","pct":%d,"ts":%d}\n' "$1" "$2" "$(date +%s)" > "$PROGRESS_FILE"
  echo "STEP:$1"
}

rm -f /opt/helios/data/.update-requested 2>/dev/null || true

progress "fetch" 5
VER="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\(v\?[0-9][^"]*\)".*/\1/p' | head -n1)"
[ -n "$VER" ] || { log "no se pudo resolver release latest"; progress "error" 0; exit 4; }
VER_NO_V="$(printf '%s' "$VER" | sed 's/^v//')"

if [ -f "$MARKER" ] && [ "$(cat "$MARKER" 2>/dev/null || true)" = "$VER_NO_V" ]; then
  log "al día ($VER_NO_V)"; rm -f "$PROGRESS_FILE"; exit 0
fi

progress "download" 15
TARBALL="helios_${VER_NO_V}_linux_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VER"
curl -fL "$BASE/$TARBALL" -o "$TMP_DIR/app.tar.gz"
progress "download" 55
TS="$(date +%s)"
curl -fL "$BASE/checksums.txt?nc=$TS" -o "$TMP_DIR/checksums.txt"

progress "verify" 60
expected="$(awk -v f="$TARBALL" '$0 ~ f {print $1; exit}' "$TMP_DIR/checksums.txt")"
[ -n "$expected" ] || { log "checksums.txt sin entrada para $TARBALL"; progress "error" 0; exit 5; }
got="$(sha256sum "$TMP_DIR/app.tar.gz" | awk '{print $1}')"
[ "$expected" = "$got" ] || { log "SHA256 NO coincide ($TARBALL)"; progress "error" 0; exit 5; }
progress "verify" 70

progress "install" 72
mkdir -p "$TMP_DIR/pkg"
tar -xzf "$TMP_DIR/app.tar.gz" -C "$TMP_DIR/pkg"

TS="$(date +%Y%m%d-%H%M%S)"
[ -d "$OPT_DIR/server/src" ] && cp -a "$OPT_DIR/server/src" "$OPT_DIR/server/src.bak-$TS"
[ -d "$OPT_DIR/shared" ] && cp -a "$OPT_DIR/shared" "$OPT_DIR/shared.bak-$TS"
[ -d "$OPT_DIR/public" ] && cp -a "$OPT_DIR/public" "$OPT_DIR/public.bak-$TS"

rm -rf "$OPT_DIR/server/src"
cp -a "$TMP_DIR/pkg/server/src" "$OPT_DIR/server/src"
[ -f "$TMP_DIR/pkg/server/package.json" ] && \
  install -m 0644 "$TMP_DIR/pkg/server/package.json" "$OPT_DIR/server/package.json"
if [ -d "$TMP_DIR/pkg/server/node_modules" ]; then
  rm -rf "$OPT_DIR/server/node_modules"
  cp -a "$TMP_DIR/pkg/server/node_modules" "$OPT_DIR/server/node_modules"
fi

rm -rf "$OPT_DIR/shared"
cp -a "$TMP_DIR/pkg/shared" "$OPT_DIR/shared"
rm -rf "$OPT_DIR/public"
mkdir -p "$OPT_DIR/public"
cp -a "$TMP_DIR/pkg/dist/." "$OPT_DIR/public/"

chown -R "$APP:$APP" "$OPT_DIR/server" "$OPT_DIR/shared" "$OPT_DIR/public"
chown "$APP:$APP" "$OPT_DIR/server/.env" 2>/dev/null || true

progress "install" 88

progress "restart" 92
printf '%s' "$VER_NO_V" > "$MARKER"
chmod 0644 "$MARKER"
if [ "${SKIP_RESTART:-0}" != "1" ]; then
  systemctl restart "$SERVICE_NAME"
fi
log "actualizado a $VER_NO_V"

progress "done" 100
