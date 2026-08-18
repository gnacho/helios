#!/bin/sh
# personal-byd.sh — despliega Helios = release oficial + overlay personal (BYD).
#
# La rama personal/byd NUNCA se mergea a main: main (o la tag indicada) se
# mergea EN ELLA, y el despliegue sale siempre de esta rama.
#
# Uso:
#   deploy/personal-byd.sh [ref]
#
#   ref = lo que se integra antes de desplegar (por defecto origin/main;
#         usar una tag, p.ej. v0.8.12, para integrar solo releases).
#
# Si el merge tiene conflictos el script se detiene: resolverlos en el
# worktree, commit, y relanzar. En server/package-lock.json lo normal es
# quedarse con la de main y regenerar:
#   git checkout --theirs server/package-lock.json
#   (cd server && npm install)   # regenera el lock con las deps propias
#
# El CT tiene el auto-update DESACTIVADO (timer+path) precisamente para que
# una release oficial nunca pise este overlay: las actualizaciones entran
# SOLO a través de este script.
set -eu

REPO="$HOME/Documentos/Repos/helios-git"
WT="$HOME/Documentos/Repos/helios-byd"
BRANCH="personal/byd"
REF="${1:-origin/main}"

GATEWAY="root@192.168.1.101"   # host Proxmox del CT
CT="226"
OPT="/opt/helios"

log() { printf '\n== %s\n' "$*"; }

log "fetch"
git -C "$REPO" fetch origin --tags --prune

if ! git -C "$REPO" worktree list | grep -q "$WT"; then
  log "creando worktree $WT"
  git -C "$REPO" worktree add "$WT" "$BRANCH"
fi
cd "$WT"
[ "$(git branch --show-current)" = "$BRANCH" ] || git checkout "$BRANCH"

log "merge de $REF en $BRANCH"
if ! git merge --no-edit "$REF"; then
  echo "CONFLICTOS: resuélvelos en $WT, commitea y relanza el script" >&2
  exit 3
fi

log "tests server"
(cd server && npm install --no-audit --no-fund && npm test)

log "build app"
(cd app && npm install --no-audit --no-fund && npx tsc --noEmit && npm run build)

VERSION="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' app/package.json | head -1)"
BUNDLE="$(cd app/dist/assets && ls index-*.js | head -1)"
LOCAL_MD5="$(md5sum "app/dist/assets/$BUNDLE" | awk '{print $1}')"

log "empaquetando v$VERSION ($BUNDLE)"
STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT
mkdir -p "$STAGING/server" "$STAGING/app"
cp -a server/src "$STAGING/server/src"
cp server/package.json "$STAGING/server/package.json"
cp app/package.json "$STAGING/app/package.json"
cp -a shared "$STAGING/shared"
cp -a app/dist "$STAGING/public"
tar -czf "$STAGING/helios-personal.tgz" -C "$STAGING" server app shared public

cat > "$STAGING/remote.sh" <<'EOF'
#!/bin/sh
set -eu
VER="$1"
TS="$(date +%Y%m%d-%H%M)"
cd /opt/helios
tar -czf "/tmp/src.bak-personal-$TS.tgz" server/src shared
tar -czf "/tmp/public.bak-personal-$TS.tgz" public
cp data/helios.db "/tmp/helios.db.bak-personal-$TS"
rm -rf /tmp/helios-new
mkdir -p /tmp/helios-new
tar -xzf /tmp/helios-personal.tgz -C /tmp/helios-new
cp -a /tmp/helios-new/server/src/. server/src/
cp /tmp/helios-new/server/package.json server/package.json
cp /tmp/helios-new/app/package.json app/package.json
cp -a /tmp/helios-new/shared/. shared/
cp -a /tmp/helios-new/public/. public/
chown -R helios:helios server/src server/package.json app/package.json shared public
echo "$VER" > .release-id
chown helios:helios .release-id
systemctl restart helios
sleep 3
systemctl is-active helios
curl -s localhost:80/api/version
EOF

log "deploy al CT $CT (via $GATEWAY)"
scp -q "$STAGING/helios-personal.tgz" "$GATEWAY:/tmp/helios-personal.tgz"
ssh -o BatchMode=yes "$GATEWAY" "pct push $CT /tmp/helios-personal.tgz /tmp/helios-personal.tgz"
scp -q "$STAGING/remote.sh" "$GATEWAY:/tmp/helios-personal-remote.sh"
ssh -o BatchMode=yes "$GATEWAY" "pct push $CT /tmp/helios-personal-remote.sh /tmp/helios-personal-remote.sh && pct exec $CT -- sh /tmp/helios-personal-remote.sh '$VERSION'"

log "verificando bundle desplegado"
REMOTE_MD5="$(ssh -o BatchMode=yes "$GATEWAY" "pct exec $CT -- md5sum $OPT/public/assets/$BUNDLE | awk '{print \$1}'")"
if [ "$REMOTE_MD5" != "$LOCAL_MD5" ]; then
  echo "FALLO: md5 del bundle difiere (local $LOCAL_MD5, CT $REMOTE_MD5)" >&2
  exit 4
fi
echo "bundle OK ($LOCAL_MD5)"

if [ -n "$(git log origin/$BRANCH..$BRANCH --oneline 2>/dev/null)" ]; then
  log "push de $BRANCH (backup)"
  git push origin "$BRANCH"
fi

log "LISTO: Helios v$VERSION + overlay BYD desplegado y verificado"
