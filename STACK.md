# Stack tecnológico Helios — referencia para otras apps

## Arquitectura
Single-page app React (Vite) servida por un backend Node.js ligero en el mismo contenedor,
sin Docker. Backend lee Home Assistant en local (WebSocket), expone API REST + SSE al
frontend por el mismo puerto, y NPM (Nginx Proxy Manager) pone SSL delante.

## Backend — Node 20, sin TypeScript, sin framework pesado
- **Servidor HTTP**: Hono + `@hono/node-server` (≈ Express pero más ligero y con tipado)
- **WebSocket HAOS**: `ws` + protocolo nativo de Home Assistant (`subscribe_entities`, `recorder/statistics_during_period`). Cliente propio en `ha.js` con reconexión automática y RPC por id
- **SQLite**: `better-sqlite3` (síncrono, rapidísimo, prebuilt por plataforma). Sin ORM: queries directas + WAL + migraciones `ALTER TABLE` tolerantes. Retiene el histórico propio para no depender de la purga de HA
- **Auth**: cookie `id.hmac` httpOnly 30d, sesiones en SQLite, `Secure` condicional por `x-forwarded-proto`, rate-limit de login en memoria
- **SSE**: `streamSSE` de Hono para push de datos en vivo al frontend, con heartbeat y throttle
- **Despliegue**: systemd (`helios.service`) con usuario dedicado + `AmbientCapabilities=CAP_NET_BIND_SERVICE` para :80 sin root

## Frontend — React 18, Vite, Tailwind + shadcn/ui (solo componentes)
- **Build**: `vite build` con sourcemaps y SPA fallback
- **Componentes**: 40+ de shadcn/ui (dropdown, tabs, accordion, etc.) + `framer-motion` para animaciones + `recharts` para gráficas + `gsap` para el arco solar
- **Estilo**: Tailwind CSS v3 con tema claro/oscuro gestionado via CSS variables + clase `dark`
- **Provider de datos**: contrato síncrono (`EnergyDataApi`) con cachés en refs y fetch perezoso. Los componentes consumen getters sin conocer HTTP. Eventos en vivo vía SSE. `suncalc` para amanecer/atardecer local si el usuario elige ubicación en Ajustes
- **Auth**: `AuthGate` → `Login` (página propia). Evento `helios-unauthorized` para volver al login automáticamente en cualquier 401

## Datos (fuentes en Home Assistant)
- **En vivo**: `subscribe_entities` a los sensores de potencia (Solis, Fox pinza, 3 medidores Tongou, batería, scraper, sun, weather, temperatura exterior)
- **Curva del día (5 min)**: `recorder/statistics_during_period` (`types:["mean"]`), retención ~10 días
- **Histórico diario**: `recorder/statistics_during_period` (`types:["state","sum"]`, `period:"day"`)

## Lecciones clave para replicar
1. Conectar a HAOS por WebSocket, no por REST polling
2. Para UMs diarias: diff de `sum`, nunca `state`
3. `X-Accel-Buffering: no` imprescindible en SSE detrás de NPM
4. Regla de sanidad: caps físicos por campo + offsets para glitches de contadores
5. Verificar el frontend contra la API real con navegador headless (Playwright), no solo curl
6. Systemd + CAP_NET_BIND_SERVICE en vez de Docker para LXCs dedicadas

Ver `ARCHITECTURE.md` para el detalle completo de cada capa y contrato.
