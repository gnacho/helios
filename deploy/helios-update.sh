#!/bin/sh
# helios-update.sh — aplica (o comprueba con --check) la última release ESTABLE de Helios.
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
#   - backup del binario y del marker anteriores
#   - sustitución atómica del binario (mv sobre el mismo filesystem)
#   - el marker solo se actualiza cuando el servicio responde sano, así que un
#     rollback devuelve binario y versión al mismo estado
#   - nunca instala una versión menor o igual a la instalada
set -eu

usage() {
  cat <<'EOF'
Uso: helios-update.sh [--check]

  --check  dice si hay una release mas nueva, sin descargar ni instalar nada
  -h       esta ayuda
EOF
}

CHECK=0
for arg in "$@"; do
  case "$arg" in
    --check) CHECK=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "opcion desconocida: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

APP=helios
REPO=gnacho/helios
ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
OPT_DIR=/opt/helios
STATE_DIR=/opt/helios/data
MARKER="$OPT_DIR/.release-id"
ENV_FILE="$OPT_DIR/server/.env"
BIN="$OPT_DIR/server/helios-go"
SERVICE_NAME=helios-go
# El progreso lo escribe root, así que vive fuera del directorio de datos del
# servicio: si estuviera dentro, un usuario del servicio podría plantar un
# symlink y hacer que root escribiera en otra ruta.
PROGRESS_DIR="$OPT_DIR/.update-state"
PROGRESS_FILE="$PROGRESS_DIR/update-progress.json"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# Binario y backups pertenecen a root: el servicio solo necesita leer y ejecutar.
# Si el usuario del servicio pudiera escribir el binario o un backup, un fallo
# en Helios se convertiria en ejecucion arbitraria como root en el siguiente
# arranque o en un rollback.
ROOT_OWNED_MODE=0755
BACKUP_MODE=0700
RESTART_DELAY=60

# Límites de red: ninguna descarga debe poder colgar el updater (TimeoutStartSec=600).
# Transporte fijo a HTTPS con pocos redirects: una redirección a http:// o a otro
# host no puede degradar la descarga silenciosamente.
CURL_NET="--proto =https --proto-redir =https --max-redirs 3"
API_MAX_TIME=30
API_MAX_BYTES=1048576
DOWNLOAD_MAX_TIME=120
DOWNLOAD_MAX_BYTES=268435456
CHECKSUMS_MAX_BYTES=65536

log() { logger -t "$APP-update" "$@"; }

progress() {
  _line="$(printf '{"step":"%s","pct":%d,"ts":%d}' "$1" "$2" "$(date +%s)")"
  mkdir -p -m 0755 "$PROGRESS_DIR"
  # Escape de symlink: si el destino ya es un enlace o no es regular, se
  # descarta antes de escribir. El temporal nace con O_EXCL por el nombre
  # aleatorio y se instala con mv atómico en el mismo sistema de archivos.
  if [ -L "$PROGRESS_FILE" ] || { [ -e "$PROGRESS_FILE" ] && [ ! -f "$PROGRESS_FILE" ]; }; then
    rm -f -- "$PROGRESS_FILE"
  fi
  _tmp="$(mktemp "$PROGRESS_DIR/.progress.XXXXXX")"
  printf '%s\n' "$_line" > "$_tmp"
  chmod 0644 "$_tmp"
  mv -f -- "$_tmp" "$PROGRESS_FILE"
  # El servidor lee el progreso desde el directorio de datos. El enlace vive
  # en el directorio escribible por el servicio y apunta al fichero root-owned,
  # nunca al revés, así que un symlink plantado no redirige la escritura.
  mkdir -p -m 0755 "$STATE_DIR"
  if [ -L "$STATE_DIR/update-progress.json" ] || [ -e "$STATE_DIR/update-progress.json" ]; then
    rm -f -- "$STATE_DIR/update-progress.json"
  fi
  ln -sfn "$PROGRESS_FILE" "$STATE_DIR/update-progress.json"
  echo "STEP:$1"
}

# Compara dos versiones X.Y.Z. Devuelve 0 cuando la primera es estrictamente
# mayor que la segunda. Cadenas no numéricas se consideran no comparables.
ver_is_newer() {
  _new="$1"
  _old="$2"
  [ -n "$_new" ] && [ -n "$_old" ] || return 1
  _on="$(printf '%s' "$_old" | tr -d 'v')"
  _nn="$(printf '%s' "$_new" | tr -d 'v')"
  _a="$(printf '%s' "$_nn" | cut -d. -f1)"
  _b="$(printf '%s' "$_nn" | cut -d. -f2)"
  _c="$(printf '%s' "$_nn" | cut -d. -f3)"
  _x="$(printf '%s' "$_on" | cut -d. -f1)"
  _y="$(printf '%s' "$_on" | cut -d. -f2)"
  _z="$(printf '%s' "$_on" | cut -d. -f3)"
  for _part in "$_a" "$_b" "$_c" "$_x" "$_y" "$_z"; do
    case "$_part" in
      ''|*[!0-9]*) return 1 ;;
    esac
  done
  _a=${_a:-0}; _b=${_b:-0}; _c=${_c:-0}
  _x=${_x:-0}; _y=${_y:-0}; _z=${_z:-0}
  if [ "$_a" -ne "$_x" ]; then [ "$_a" -gt "$_x" ]; return $?; fi
  if [ "$_b" -ne "$_y" ]; then [ "$_b" -gt "$_y" ]; return $?; fi
  [ "$_c" -gt "$_z" ]
}

# El health check no puede asumir el puerto 80: Helios escucha en PORT y detrás
# de un proxy el puerto publicado puede ser otro.
health_port() {
  _port=""
  if [ -r "$ENV_FILE" ]; then
    _port="$(sed -n 's/^[[:space:]]*PORT[[:space:]]*=[[:space:]]*//p' "$ENV_FILE" | tail -n1 | tr -d '"'"'"'[:space:]')"
  fi
  case "$_port" in
    ''|*[!0-9]*) _port=${PORT:-8080} ;;
  esac
  printf '%s' "$_port"
}

rm -f "$STATE_DIR/.update-requested" 2>/dev/null || true

VER="$(curl -fsSL $CURL_NET --max-time "$API_MAX_TIME" --max-filesize "$API_MAX_BYTES" \
  "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\(v\?[0-9][^"]*\)".*/\1/p' | head -n1)"
if [ -z "$VER" ]; then
  log "no se pudo resolver release latest"
  [ "$CHECK" = "1" ] || progress "error" 0
  echo "error: no se pudo consultar la ultima release" >&2
  exit 4
fi
VER_NO_V="$(printf '%s' "$VER" | sed 's/^v//')"
CURRENT="$(cat "$MARKER" 2>/dev/null || true)"

# --check: solo informa, no toca el estado del actualizador.
if [ "$CHECK" = "1" ]; then
  if [ -z "$CURRENT" ]; then
    echo "instalada: desconocida · disponible: $VER_NO_V"
  elif [ "$CURRENT" = "$VER_NO_V" ]; then
    echo "al dia: $CURRENT"
  else
    echo "disponible: $VER_NO_V (instalada: $CURRENT)"
  fi
  exit 0
fi

progress "fetch" 5

if [ -n "$CURRENT" ] && [ "$CURRENT" = "$VER_NO_V" ]; then
  log "al día ($VER_NO_V)"; rm -f "$PROGRESS_FILE"; exit 0
fi

# Nunca degradar: una release latest anterior a la instalada es un error, no una actualización.
if [ -n "$CURRENT" ] && ! ver_is_newer "$VER_NO_V" "$CURRENT"; then
  log "release $VER_NO_V no es mas nueva que la instalada $CURRENT: se ignora"
  progress "error" 0
  echo "error: la release disponible ($VER_NO_V) no es mas nueva que la instalada ($CURRENT)" >&2
  exit 7
fi

progress "download" 15
BIN_NAME="helios_${VER_NO_V}_linux_${ARCH}"
BASE="https://github.com/$REPO/releases/download/$VER"
curl -fsSL $CURL_NET --max-time "$DOWNLOAD_MAX_TIME" --max-filesize "$DOWNLOAD_MAX_BYTES" \
  "$BASE/$BIN_NAME" -o "$TMP_DIR/helios-go"
progress "download" 55
TS="$(date +%s)"
curl -fsSL $CURL_NET --max-time "$DOWNLOAD_MAX_TIME" --max-filesize "$CHECKSUMS_MAX_BYTES" \
  "$BASE/checksums.txt?nc=$TS" -o "$TMP_DIR/checksums.txt"

progress "verify" 60
EXPECTED_BIN="$(awk -v f="$BIN_NAME" '$0 ~ f {print $1; exit}' "$TMP_DIR/checksums.txt")"
[ -n "$EXPECTED_BIN" ] || { log "checksums.txt sin entrada para $BIN_NAME"; progress "error" 0; exit 5; }
GOT_BIN="$(sha256sum "$TMP_DIR/helios-go" | awk '{print $1}')"
[ "$EXPECTED_BIN" = "$GOT_BIN" ] || { log "SHA256 binario NO coincide"; progress "error" 0; exit 5; }
progress "verify" 70

progress "install" 72
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP_BIN="$OPT_DIR/server/helios-go.bak-$TS"
BACKUP_MARKER=""
if [ -f "$MARKER" ]; then
  BACKUP_MARKER="$OPT_DIR/.release-id.bak-$TS"
  cp -a "$MARKER" "$BACKUP_MARKER"
  chmod 0600 "$BACKUP_MARKER"
fi
if [ -f "$BIN" ]; then
  cp -a "$BIN" "$BACKUP_BIN"
  chmod "$BACKUP_MODE" "$BACKUP_BIN"
fi

# Binario: copia al lado con propietario root y mv atómico. El usuario del
# servicio nunca puede escribir el binario que se ejecuta ni los backups.
install -m "$ROOT_OWNED_MODE" -o root -g root "$TMP_DIR/helios-go" "$OPT_DIR/server/helios-go.new"

progress "install" 88
mv -f "$OPT_DIR/server/helios-go.new" "$BIN"
chown root:root "$BIN"
chmod "$ROOT_OWNED_MODE" "$BIN"
# El directorio del binario queda escribible solo por root: el servicio no puede
# introducir un binario nuevo donde systemd lo ejecutaria como root al reiniciar.
if [ -d "$OPT_DIR/server" ]; then chmod 0750 "$OPT_DIR/server"; fi

progress "restart" 92

if [ "${SKIP_RESTART:-0}" != "1" ]; then
  systemctl restart "$SERVICE_NAME"
  # Espera a que arranque y responde con el nuevo binario antes de declarar la versión.
  HPORT="$(health_port)"
  OK=0
  i=0
  while [ $i -lt 15 ]; do
    sleep 1
    if curl -fsS --max-time 3 "http://127.0.0.1:$HPORT/health" >/dev/null 2>&1; then OK=1; break; fi
    i=$((i+1))
  done
  if [ "$OK" != "1" ]; then
    log "el servicio no respondió tras actualizar: ROLLBACK a la versión anterior"
    if [ -f "$BACKUP_BIN" ]; then
      mv -f "$BACKUP_BIN" "$BIN"
      if [ -n "$BACKUP_MARKER" ] && [ -f "$BACKUP_MARKER" ]; then
        mv -f "$BACKUP_MARKER" "$MARKER"
      else
        rm -f "$MARKER"
      fi
      systemctl restart "$SERVICE_NAME" || true
    fi
    progress "error" 0
    exit 6
  fi
fi

# La versión instalada solo se declara cuando el servicio ya está sano.
printf '%s' "$VER_NO_V" > "$MARKER"
chmod 0644 "$MARKER"
[ -n "$BACKUP_MARKER" ] && rm -f "$BACKUP_MARKER"

log "actualizado a $VER_NO_V"
progress "done" 100
