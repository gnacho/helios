# Changelog

Todos los cambios notables de Helios se documentan en este fichero.

El formato se basa en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/),
y este proyecto se adhiere a [Versionado Semántico](https://semver.org/lang/es/).

## [Unreleased]

### Changed

- **Migración del toolchain de build**: Vite 7 → 8 (Rolldown), @vitejs/plugin-react 5 → 6, Tailwind CSS 3 → 4 (config en CSS vía `@theme` + `tw-animate-css`, plugin Vite `@tailwindcss/vite`).
- **React Router 7 → 8.3.0**: imports migrados de `react-router-dom` a `react-router`.
- **Backend**: @hono/node-server 1.x → 2.1.0.

### Fixed

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
