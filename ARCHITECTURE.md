# Arquitectura Helios — referencia para replicar

Patrón completo "app web local sobre Home Assistant": backend Go + SQLite (Node/Hono hasta v0.8.x,
retirado del árbol en v1.0.0), pantallas renderizadas en el servidor con templ + Datastar
y todo embebido en un único binario. Pensado para ser reproducido tal cual en otras apps del entorno.

## 1. Stack y layout

```
server-go/
  cmd/server/main.go   # entry: carga la configuración y arranca HA + HTTP + jobs
  internal/api/        # API REST/SSE (mismo contrato que el Node)
    server.go          # rutas /api/*, /health y mount de la UI templ
  internal/auth/       # login, sesiones en cookie, rate-limit
  internal/db/         # modernc.org/sqlite (puro Go): schema + helpers + migraciones
  internal/ha/         # cliente WebSocket HAOS (subscribe_entities + RPC)
  internal/solar/      # lógica de negocio (live, series, KPIs, backfill, consolidación)
  internal/install/    # topología del cliente (inversores, batería, entidades)
  internal/extensions/ # cargadores, precios, fotos, curva del día
  internal/push/       # Web Push (VAPID)
  internal/sse/        # hub de SSE (pool API y pool Datastar)
  internal/web/        # UI server-rendered (templ + Datastar) + assets embebidos
    assets/root/       # favicon, iconos PWA, manifest.webmanifest, sw.js (embed)
    assets/css|js/     # app.css -> app.build.css, datastar.js, gestures.js
    locales/           # catálogos i18n (es, en, zh-CN), fuente única
deploy/               # unidad systemd + helios-update.sh (descarga el binario y reinicia)
landing/              # web de marketing del proyecto (estática)
```

Un solo binario estático (sin CGO) sirve la API, la UI y los estáticos raíz: no hay
`public/`, ni `STATIC_DIR`, ni build de frontend que copiar al servidor. La UI se
genera con templ y el CSS con el binario standalone de Tailwind (`make build`), así
que el árbol no necesita Node. Despliegue: systemd con usuario dedicado +
`AmbientCapabilities=CAP_NET_BIND_SERVICE` para escuchar en :80 sin root. NPM
(Nginx Proxy Manager) delante con SSL.

## 2. Conexión con Home Assistant (`ha.ts`)

WebSocket persistente a `ws://<haos>/api/websocket`, protocolo nativo HA:

1. Conectar → server envía `auth_required` → enviamos `{type:"auth", access_token}`.
2. `auth_ok` → suscribir entidades: `{id, type:"subscribe_entities", entity_ids:[...]}`.
3. Primer evento `event.a` = snapshot completo (`{s: state, a: attributes, lu}`).
   Siguientes: `event.c` con diffs (`{"+": {...}}` / `"-"`).
4. Llamadas RPC: `call({type:"recorder/statistics_during_period", ...})` con `id`
   incremental; respuestas `type:"result"` resueltas por id (Map de pending + timeout 60s).
5. Reconexión: backoff exponencial 1s→30s; al reconectar re-suscribe.
6. `auth_invalid` → fatal, no reintentar.

El cliente mantiene `Map<entity_id, {state, attributes, lastUpdated}>` y emite
`entity` por cada cambio. Todo el resto de la app lee de ahí (cero polling REST).

Series históricas: `recorder/statistics_during_period`:
- `period:"5minute", types:["mean"]` → curvas de potencia del día (retención ~10 días).
- `period:"day", types:["state","sum"]` → histórico diario.
  - Contadores lifetime (pinzas, medidores): diario = **diff de `state`**.
  - Utility meters / templates diarios: diario = **diff de `sum`** (el `state` de la
    fila diaria es el valor a las 00:00 = 0 — trampa importante).

## 3. Backend HTTP (`index.ts`)

```
POST /api/auth/login      user+pass timingSafeEqual → cookie helios_session=id.hmac
POST /api/auth/logout
GET  /api/auth/me
GET  /api/solar/live      computeLive() + alertas
GET  /api/solar/day?date  288 puntos 5-min + flag estimated
GET  /api/solar/kpis?date hoy (live) o pasado (SQLite daily)
GET  /api/solar/history   días desde SQLite (from/to)
POST /api/solar/history/refresh  re-backfill manual
GET  /api/solar/stream    SSE: push live throttled 1s + heartbeat 25s (mensaje
                          data {"type":"ping"}; el comentario `: ping` lo ignora EventSource)

## Alertas push (v0.3)

- Motor `server/src/alerts.ts` (tick 60 s, flancos + anti-rebote): inversor_offline
  (solo de día + scraper fresco), fox_offline (pinza unavailable solo de día),
  corte_red (heurística isla), bateria_baja (SOC ≤ reserva), resumen_diario al
  anochecer (sun.sun next_setting + offset, fallback 21:00 sin dato solar).
- Web Push VAPID (`push.ts`, `routes-push.ts`): prefs por tipo, quiet hours con
  cola consolidada, i18n server-side es/en/zh. Sin VAPID arranca con push off.
- Toda alerta disparada se audita (`audit_log`, action `alert:<tipo>`) para
  ajustar umbrales con historial real.
- Migraciones de esquema: `user_version` en db.ts (v1 daily solis/fox, v2 sessions
  viejo). Los CREATE IF NOT EXISTS son idempotentes; las transformaciones van en
  migraciones numeradas, nunca editadas a posteriori.
GET/PUT /api/config       ajustes instalación (kv)
```

- Auth: middleware en `/api/*` (menos login). Cookie `id.hmac` httpOnly, SameSite=Lax,
  30 días, `Secure` solo si `x-forwarded-proto=https` (compatible LAN http + NPM https).
  Secret HMAC: env o autogenerado persistido en `kv` (sobrevive reinicios).
  Rate-limit login: 5 fallos → 5 min por IP (en memoria).
- SSE: `streamSSE` de Hono; cabeceras `X-Accel-Buffering: no` y `Cache-Control: no-cache`
  (imprescindible detrás de nginx/NPM para no bufferar).
- Static: `serveStatic(root)` + fallback GET `*` → `index.html` excepto `/api/*` y `/assets/*`.
- Jobs: consolidación nocturna 00:10 (backfill diario + baseline consumo),
  limpieza de sesiones cada hora.

## 4. SQLite (`db.ts`)

```sql
daily(date PK, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh,
      battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh)
sessions(id PK, created_at, expires_at, ua)
kv(key PK, value)          -- session_secret, cons_baseline, install_config
```

- `journal_mode = WAL`. Migraciones tolerantes: `PRAGMA table_info` + `ALTER TABLE ADD COLUMN` si falta.
- El histórico vive AQUÍ (no depende de la retención de HA): backfill inicial desde
  statistics de HA + consolidación nocturna del día anterior.
- Reglas de sanidad al importar: caps físicos por campo y offsets de glitches conocidos
  de contadores (ver `FOX_GLITCH_OFFSETS`).

## 5. Datos en el cliente (Datastar, sin SPA)

La UI no tiene capa de datos propia: el servidor manda HTML y parchea fragmentos.

- **Live**: cada página declara `data-init="@get('/ui/stream')"`; el handler abre un
  stream Datastar y emite `patch-signals` con el snapshot (`live`, `chg.*`, `ui.*`) y un
  `: ping` cada 25 s. En el navegador eso son señales que los `data-text`/`data-attr`
  leen sin JavaScript de aplicación.
- **Navegación y pestañas**: los enlaces llevan `data-on:click__prevent` que hace
  `history.pushState` + `@get('/ui/nav' | '/ui/chg' | '/ui/inv' | '/ui/historico')`; el
  servidor reutiliza el handler de la página con `httptest.ResponseRecorder`, extrae el
  fragmento entre marcadores (`<!--#page-->`) y lo parchea. Sin recarga del documento.
- **Formularios**: `@post`/`@get` con señales (`data-bind`) y autoguardado; el handler
  responde con el fragmento a refrescar (o 204).
- **i18n y formato**: se resuelven en el servidor (`T(lang, key)` y `fmtKw`/`fmtEnergy`),
  así que el navegador solo recibe texto final.

Las gráficas son SVG generadas en Go; el hover es una capa de bandas transparentes con
CSS puro (`chart_hover.go`).

## 6. Decisiones de datos (lecciones caras)

1. Medidores físicos siempre mejor que templates derivados (el template de consumo
   tenía bug de signo y da 0/inflado).
2. `currentGridPower` (Solis) es CON SIGNO de fábrica → `Math.abs` + campo dirección.
3. Para totales diarios de UMs: diff de `sum`, nunca `state`.
4. Sensores sin `state_class` no tienen statistics: cualquier potencia nueva que se
   quiera graficar necesita `device_class` + `state_class: measurement` en HA.
5. Consumo real de la casa = `max(medidores, balance)` con
   `balance = producción + import − export − carga + descarga` (los circuitos sin
   medidor quedan cubiertos por la derivada).
6. Detrás de NPM/nginx: SSE necesita `X-Accel-Buffering: no`.
7. Verificar el frontend contra la API real (Playwright headless), no solo curl.
