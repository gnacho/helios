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
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

log() { logger -t "$APP-update" "$@"; }

# El apply in-app escribe un flag en el dir de datos (ver update.js / el .path
# de systemd). Borrarlo AL PRINCIPIO permite re-disparar el apply a voluntad.
rm -f /opt/helios/data/.update-requested 2>/dev/null || true

echo "STEP:detect"
VER="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\(v\?[0-9][^"]*\)".*/\1/p' | head -n1)"
[ -n "$VER" ] || { log "no se pudo resolver release latest"; exit 4; }
VER_NO_V="$(printf '%s' "$VER" | sed 's/^v//')"

# Marker semver real (fuente de verdad para /api/update/status).
if [ -f "$MARKER" ] && [ "$(cat "$MARKER" 2>/dev/null || true)" = "$VER_NO_V" ]; then
  log "al día ($VER_NO_V)"; exit 0
fi

echo "STEP:download"
TARBALL="helios_${VER_NO_V}_linux_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VER"
curl -fL "$BASE/$TARBALL" -o "$TMP_DIR/app.tar.gz"
# Cache-buster: la URL de checksums.txt es estable entre versiones y la CDN
# sirve la copia vieja justo tras publicar → el tarball nuevo "no está en
# checksums". Añadir ?nc=<ts> fuerza la revalidación.
TS="$(date +%s)"
curl -fL "$BASE/checksums.txt?nc=$TS" -o "$TMP_DIR/checksums.txt"

echo "STEP:verify"
expected="$(awk -v f="$TARBALL" '$0 ~ f {print $1; exit}' "$TMP_DIR/checksums.txt")"
[ -n "$expected" ] || { log "checksums.txt sin entrada para $TARBALL"; exit 5; }
got="$(sha256sum "$TMP_DIR/app.tar.gz" | awk '{print $1}')"
[ "$expected" = "$got" ] || { log "SHA256 NO coincide ($TARBALL)"; exit 5; }

echo "STEP:extract"
mkdir -p "$TMP_DIR/pkg"
tar -xzf "$TMP_DIR/app.tar.gz" -C "$TMP_DIR/pkg"

echo "STEP:deploy"
# Backup del estado actual antes de tocar nada.
TS="$(date +%Y%m%d-%H%M%S)"
[ -d "$OPT_DIR/server/src" ] && cp -a "$OPT_DIR/server/src" "$OPT_DIR/server/src.bak-$TS"
[ -d "$OPT_DIR/shared" ] && cp -a "$OPT_DIR/shared" "$OPT_DIR/shared.bak-$TS"
[ -d "$OPT_DIR/public" ] && cp -a "$OPT_DIR/public" "$OPT_DIR/public.bak-$TS"

# Código server: src/ nuevo (conserva .env y node_modules si no cambian).
rm -rf "$OPT_DIR/server/src"
cp -a "$TMP_DIR/pkg/server/src" "$OPT_DIR/server/src"
[ -f "$TMP_DIR/pkg/server/package.json" ] && \
  install -m 0644 "$TMP_DIR/pkg/server/package.json" "$OPT_DIR/server/package.json"
if [ -d "$TMP_DIR/pkg/server/node_modules" ]; then
  rm -rf "$OPT_DIR/server/node_modules"
  cp -a "$TMP_DIR/pkg/server/node_modules" "$OPT_DIR/server/node_modules"
fi

# Shared + frontend.
rm -rf "$OPT_DIR/shared"
cp -a "$TMP_DIR/pkg/shared" "$OPT_DIR/shared"
rm -rf "$OPT_DIR/public"
mkdir -p "$OPT_DIR/public"
cp -a "$TMP_DIR/pkg/dist/." "$OPT_DIR/public/"

chown -R "$APP:$APP" "$OPT_DIR/server" "$OPT_DIR/shared" "$OPT_DIR/public"
chown "$APP:$APP" "$OPT_DIR/server/.env" 2>/dev/null || true

echo "STEP:restart"
printf '%s' "$VER_NO_V" > "$MARKER"
chmod 0644 "$MARKER"
if [ "${SKIP_RESTART:-0}" != "1" ]; then
  systemctl restart "$SERVICE_NAME"
fi
log "actualizado a $VER_NO_V"

# Rollback manual (cada dir tiene su backup .bak-$TS):
#   rm -rf /opt/helios/server/src && mv /opt/helios/server/src.bak-$TS /opt/helios/server/src \
#   && rm -rf /opt/helios/shared && mv /opt/helios/shared.bak-$TS /opt/helios/shared \
#   && rm -rf /opt/helios/public && mv /opt/helios/public.bak-$TS /opt/helios/public \
#   && systemctl restart helios
