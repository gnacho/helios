#!/bin/bash
# Script para crear nuevo servicio en CT 226
# Uso: ./crear-servicio.sh <nombre-servicio> <puerto> <ruta-local-proyecto>

set -e

if [ $# -lt 3 ]; then
    echo "Uso: $0 <nombre-servicio> <puerto> <ruta-local-proyecto>"
    echo "Ejemplo: $0 panelsolar 8080 /home/demo/Documentos/Mi Nube/Documentos/domotica/Local_Agent_PanelSolar"
    exit 1
fi

SERVICE_NAME=$1
PORT=$2
LOCAL_PATH=$3
CT_IP="192.168.10.226"
CT_USER="root"

echo "=== Creando servicio: $SERVICE_NAME en puerto $PORT ==="

# 1. Crear usuario del sistema
echo "1. Creando usuario del sistema..."
ssh $CT_USER@$CT_IP "useradd -r -s /bin/false $SERVICE_NAME 2>/dev/null || echo 'Usuario ya existe'"

# 2. Crear estructura de directorios
echo "2. Creando estructura de directorios..."
ssh $CT_USER@$CT_IP "mkdir -p /opt/$SERVICE_NAME/{server,public,data} && chown -R $SERVICE_NAME:$SERVICE_NAME /opt/$SERVICE_NAME"

# 3. Copiar código
echo "3. Copiando código..."
tar czf - server/src server/package.json server/package-lock.json | ssh $CT_USER@$CT_IP "tar xzf - -C /opt/$SERVICE_NAME"

# 4. Copiar frontend (si existe)
if [ -d "$LOCAL_PATH/app/dist" ]; then
    echo "4. Copiando frontend..."
    tar czf - app/dist | ssh $CT_USER@$CT_IP "tar xzf - -C /opt/$SERVICE_NAME && mv /opt/$SERVICE_NAME/app/dist /opt/$SERVICE_NAME/public && rmdir /opt/$SERVICE_NAME/app"
else
    echo "4. Frontend no encontrado, saltando..."
fi

# 5. Instalar dependencias
echo "5. Instalando dependencias..."
ssh $CT_USER@$CT_IP "cd /opt/$SERVICE_NAME/server && npm install --production"

# 6. Crear .env
echo "6. Creando .env..."
ssh $CT_USER@$CT_IP "cat > /opt/$SERVICE_NAME/server/.env << 'EOF'
PORT=$PORT
HAOS_URL=http://192.168.10.244:8123
HAOS_TOKEN=HAOS_TOKEN_ELIMINADO_POR_REWRITE
AUTH_USER=admin
AUTH_PASS=$(openssl rand -hex 16)
SESSION_SECRET=$(openssl rand -hex 32)
EOF
chmod 600 /opt/$SERVICE_NAME/server/.env
chown $SERVICE_NAME:$SERVICE_NAME /opt/$SERVICE_NAME/server/.env"

# 7. Crear servicio systemd
echo "7. Creando servicio systemd..."
ssh $CT_USER@$CT_IP "cat > /etc/systemd/system/$SERVICE_NAME.service << 'EOF'
[Unit]
Description=$SERVICE_NAME
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_NAME
Group=$SERVICE_NAME
WorkingDirectory=/opt/$SERVICE_NAME/server
ExecStart=/usr/bin/node src/index.js
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now $SERVICE_NAME"

# 8. Verificar estado
echo "8. Verificando estado..."
sleep 3
ssh $CT_USER@$CT_IP "systemctl status $SERVICE_NAME --no-pager"

echo ""
echo "=== Servicio $SERVICE_NAME creado exitosamente ==="
echo "Puerto: $PORT"
echo "URL local: http://$CT_IP:$PORT"
echo "Siguiente paso: Configurar proxy en NPM (CT 201) para $SERVICE_NAME.duckdns.org"
