# Arquitectura Helios — referencia para replicar

Patrón completo "app web local sobre Home Assistant": backend Node + Hono + SQLite,
frontend React con provider de datos en vivo. Pensado para ser reproducido tal cual
en otras apps del entorno.

## 1. Stack y layout

```
server/
  package.json        # type: module, node >= 20, 4 deps: hono, @hono/node-server, ws, better-sqlite3
  .env                # PORT, HAOS_URL, HAOS_TOKEN, AUTH_USER, AUTH_PASS, STATIC_DIR, DATA_DIR, precios
  src/
    index.js          # entry: carga .env manual, arranca HA + Hono + jobs
    config.js         # config por env + ENTITIES + LIVE_ENTITIES + ENERGY_ENTITIES
    ha.js             # cliente WebSocket HAOS
    solar.js          # lógica de negocio (live, series, KPIs, backfill)
    auth.js           # login/sesiones/rate-limit/middleware
    db.js             # better-sqlite3: schema + helpers
  data/helios.db      # SQLite (WAL)
  public/             # dist/ del frontend (SPA)
```

Sin ORM, sin framework de más, sin TypeScript en backend (JS plano ESM).
Despliegue: systemd con usuario dedicado + `AmbientCapabilities=CAP_NET_BIND_SERVICE`
para escuchar en :80 sin root. NPM (Nginx Proxy Manager) delante con SSL.

## 2. Conexión con Home Assistant (`ha.js`)

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

## 3. Backend HTTP (`index.js`)

```
POST /api/auth/login      user+pass timingSafeEqual → cookie helios_session=id.hmac
POST /api/auth/logout
GET  /api/auth/me
GET  /api/solar/live      computeLive() + alertas
GET  /api/solar/day?date  288 puntos 5-min + flag estimated
GET  /api/solar/kpis?date hoy (live) o pasado (SQLite daily)
GET  /api/solar/history   días desde SQLite (from/to)
POST /api/solar/history/refresh  re-backfill manual
GET  /api/solar/stream    SSE: push live throttled 1s + heartbeat 30s
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

## 4. SQLite (`db.js`)

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

## 5. Provider frontend (`app/src/data/EnergyDataProvider.tsx`)

Contrato síncrono (los componentes no conocen HTTP):

```ts
interface EnergyDataApi {
  connectionStatus: 'connected' | 'reconnecting' | 'demo';
  today: Date; now: Date; nowMin: number; liveTick: number;
  sunriseMin: number; sunsetMin: number;
  refresh(): void;                                  // limpia cachés y refetchea
  getLivePower(atMin?, tick?): LivePower;           // live (SSE) o punto del replay
  getDaySeries(date?): PowerPoint[];                // caché + fetch perezoso
  isDayEstimated(date?): boolean;
  getKpis(date?, untilMin?): DayKpis;               // caché; untilMin → integra cliente
  getHistory(range?): HistoryDay[];
}
```

Mecánica:
- **Live**: `EventSource('/api/solar/stream')` → `liveRef` + `bump()` (state `version`).
- **Cachés en refs**: `dayCache`, `dayEstimated`, `kpisCache`, `historyRef`; set `pending`
  anti-duplicados. Los getters disparan fetch si falta y devuelven lo último conocido
  (o vacío); al completar, `bump()` re-renderiza.
- **value** = `useMemo([version, now, connectionStatus, locationLat, locationLon])` con
  closures nuevas cada vez → los `useMemo` de los consumidores recomputan al cambiar datos.
- **Clock** 1s (`now`), refresco periódico 5 min (day+kpis+history de hoy).
- **401** en cualquier fetch → evento `helios-unauthorized` → `AuthGate` muestra `Login`.
- **Sol**: `suncalc` con lat/lon de Ajustes si hay; si no, `sun.sun` de HA.

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
