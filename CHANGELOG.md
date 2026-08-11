# Changelog

Todos los cambios notables de Helios se documentan en este fichero.

El formato se basa en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/),
y este proyecto se adhiere a [Versionado Semántico](https://semver.org/lang/es/).

## [Unreleased]

## [0.7.3] - 2026-08-11

### Added

- **README: conexión con HAOS documentada (issue #48)**: nueva sección Installation/Instalación — `HAOS_URL` + token de larga duración (`HAOS_TOKEN`, Perfil → Seguridad → Tokens de larga duración) y copia de `.env.example` a `.env`. Corregido el callout que decía que no había editor de topología en la app (existe desde 0.7.2) y alineada la tabla de hoja de ruta con `ROADMAP.md` (Fase 3 hecha, Fase 4 próxima).

## [0.7.1] - 2026-08-11

### Changed

- **Alertas y notificaciones sin marcas de inversor (issue #39)**: los textos de usuario ya no mencionan "Solis"/"Fox"/"Soluna". `computeLive` emite "Inversor offline" y "Datos de producción antiguos"; el catálogo push de `inversor_offline`/`inversor_ok` es genérico y `fox_offline`/`fox_ok` usan el nombre del 2º inversor desde la topología (fallback "secundario"). Textos de UI (curvas, reparto, nota de estimación, batería, footer, tipos de notificación) genéricos.
- **Ajustes → Tu instalación y Footer desde la topología**: tarjetas de inversores, kWp total y placas se construyen con `useInstall()`; el footer resume "N inversores · Batería X kWh" en vez del texto específico de la casa.

## [0.7.0] - 2026-08-11

### Added

- **Topología configurable (Fase 3 del roadmap, issue #37)**: la instalación (inversores, batería, fuente de red, mapeo de sensores de HAOS) ya no está hardcodeada. Nuevo módulo `server/src/install.js` que resuelve la topología desde `install_config` (JSON en kv) con esta prioridad: config del admin > perfil legacy para instalaciones existentes (attrs del scraper + Solis/Fox + batería en español) > perfil genérico para instalaciones nuevas (sin scraper, red por sensores planos, batería opcional).
- **N inversores**: cada uno con sus sensores de potencia/energía y unidad (kW/W). `daily` gana la columna `inverters_kwh` (migración v4, sin tocar las existentes); `solis_kwh`/`fox_kwh` siguen escribiéndose (los 2 primeros inversores) por compatibilidad.
- **Batería con estados en varios idiomas**: `chargingStates`/`dischargingStates` aceptan cualquier cadena (`'Cargando'`, `'charging'`…), ya no depende del español.
- **Grid source configurable**: `grid.mode: 'attrs'` lee la potencia/dirección de los atributos de un sensor HAOS (el scraper Solis es solo un sensor así), `grid.mode: 'sensor'` lee sensores planos (con signo o import/export). `statusAttrsId` (opcional) aporta online/station del inversor, desacoplado del grid. Todo viene de HAOS.
- **`GET /api/install`**: devuelve la topología resuelta + entidades en uso; la UI (Ajustes → Conexión) ya no tiene la lista de entidades hardcodeada.
- **Frontend genérico por topología**: página Inversores con tabs dinámicos (comparativa solo con ≥2), tabla de métricas de N columnas, reparto de N segmentos; Dashboard y DayChart se construyen desde la topología. `useInstall` (hook con caché de módulo).

### Changed

- `solar.js` (live/series/KPIs/backfill) y `alerts.js` guiados por la topología resuelta; `solis`/`fox` se mantienen como alias de los 2 primeros inversores.

## [0.6.2] - 2026-08-09

### Changed

- **`corte_red` desactivado por defecto (issue #19)**: la alerta de corte de red daba falsos críticos porque la firma diferencial no es fiable con los sensores actuales (la pinza del circuito no respaldado está casi siempre a ~0 W por consumo bajo, y el scraper no expone tensión de red ni flag de EPS). Se mantiene `inversor_offline` (apagón total) y `fox_offline`. Para rehabilitarla en el futuro: `install_config.corteRedEnabled = true`; al disparar, el `audit_log` guarda ahora los datos del disparo (gridMag, respaldoKw, noRespaldadaKw, batteryPower, fresco).

### Fixed

- **Versión del server leída del package.json propio (issue #24)**: `index.js` leía `../../app/package.json`, que no existe en el layout plano de deploy (`/opt/helios/{server,public,shared}`) → en una instalación fresca el servidor no arrancaba, y en el CT daba una versión residual (0.5.0). Ahora la versión y el nombre salen del propio `server/package.json` (sincronizado con la app) y la versión de React se lee del frontend con fallback a `''`.
- **Migración del toolchain de build**: Vite 7 → 8 (Rolldown), @vitejs/plugin-react 5 → 6, Tailwind CSS 3 → 4 (config en CSS vía `@theme` + `tw-animate-css`, plugin Vite `@tailwindcss/vite`).
- **React Router 7 → 8.3.0**: imports migrados de `react-router-dom` a `react-router`.
- **Backend**: @hono/node-server 1.x → 2.1.0.

### Fixed

- **Login fallido mostraba "Sesión expirada" (issue #14)**: el manejador global de 401 trataba todo 401 como sesión caída y descartaba el mensaje real. `/api/auth/login` queda excluido del manejador global y `Login.tsx` mapea el 401 al mensaje localizado de credenciales incorrectas.
- **Vulnerabilidad HIGH (CSRF en modo RSC)**: GHSA-qwww-vcr4-c8h2, afectaba a react-router 7.12.0–8.2.0; cerrada con react-router 8.3.0.
- **Backfill con huecos de datos**: un día sin fila de estadísticas en HAOS concentraba el delta del acumulador en el día posterior (varios días de consumo en uno). Ahora el delta se reparte uniformemente entre los días del hueco.
- **SSE: limpieza de clientes zombies**: si el `write` del heartbeat falla (cliente caído sin disparar `onAbort`), el cliente se elimina del set y el intervalo se detiene (antes se quedaba ocupando un slot de `MAX_SSE_CLIENTS`).

## [0.6.0] - 2026-08-01

### Added

- Repo público: ribbon de actualización semanal, releases semver y CSP.
- AdminBar canónica: Actualizaciones → Usuarios → Auditoría, con audit log.

### Changed

- Saneados los datos personales del historial y licencia AGPL-3.0.

## [0.5.0] - 2026-07-30

### Added

- Gestión de usuarios con roles (creación, password, idioma, rol, borrado).
- i18n completo de la app (es/en/zh-CN) con fechas y números localizados.
- Idioma manual sincronizado al perfil y modo 'auto' persistente.
- Iconos de marca duales claro/oscuro (favicon, PWA, logo).
- Multiusuario, cliente HTTP centralizado (401 anti-cascada) y logout por evento.
- Cache TTL + single-flight en statistics de HAOS y validación de config con zod.
- `/health`, límite de clientes SSE, checkpoint WAL horario y paginación en `/api/solar/history`.
- Seguridad: headers HTTP (HSTS, CSP, X-Frame-Options), rate-limit en SQLite, bcrypt para AUTH_PASS, validación zod de entrada, graceful shutdown.
- Ajustes sin mini-nav a todo ancho; sección Seguridad con cambio de contraseña propia.
- Bootstrap del admin inicial desde `.env`.

### Fixed

- Rate-limit bloquea en el 5.º intento fallido.
- Login en instalación fresca sin cuenta admin.
- `login_attempts` creada en el schema (el rate-limit daba 500 sin ella).
- Import de bcrypt que faltaba en el endpoint de cambio de password.
- Flujo inline del logo (currentColor) y redirección a `/` tras login.
