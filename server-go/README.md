# helios-server (Go)

Backend Go para Helios. Experiment: reemplaza el backend Node/Hono manteniendo el contrato API con el frontend React.

## Estado

Scaffold inicial. Incluye:
- `net/http` con mux.
- SQLite vía `mattn/go-sqlite3` (WAL, foreign keys, single writer como better-sqlite3).
- Migraciones mínimas (users, sessions, kv).
- Health check en `/health`.

## Desarrollo

```bash
go mod tidy
go build ./cmd/server
HELIOS_DATA_DIR=/tmp/helios-data ./helios-server
```

## Estructura

- `cmd/server/main.go`: entrypoint, wiring.
- `internal/config`: env vars (PORT, HELIOS_DATA_DIR, HELIOS_DB_PATH, TRUSTED_PROXIES).
- `internal/db`: apertura SQLite + migraciones.
- `internal/health`: handler `/health`.

## Pendiente

- Portar auth (bcrypt, sessions, login/register/logout).
- Portar API solar (daily, day_series, SSE).
- Portar HAOS client (WebSocket).
- Portar push (Web Push VAPID).
- Portar alerts y extensions.
