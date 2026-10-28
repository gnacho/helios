# helios-server (Go)

Backend de Helios en Go. Sustituye al backend Node/Hono desde v1.0.0 y sirve la API y la UI desde un único binario estático.

## Estado

En producción. El binario reúne:

- API REST/SSE en `/api/*`.
- Autenticación multiusuario con bcrypt, sesiones en cookie y límite de intentos de login.
- Cliente WebSocket de Home Assistant, con suscripción de entidades y llamadas a `recorder/statistics_during_period`.
- SQLite con `modernc.org/sqlite` (Go puro, sin CGO): WAL, foreign keys, `busy_timeout` de 5 s y una sola conexión abierta.
- Alertas, Web Push con VAPID, auto-actualización y extensiones (cargadores, BYD).
- UI renderizada en el servidor con templ + Datastar, embebida en el binario junto con el CSS, el JS y los estáticos raíz.
- `/health` responde `{"status":"ok"}` y `/api/version` la versión con el identificador de build.

No queda nada del scaffold. La UI React se retiró, así que no hay `public/`, ni `STATIC_DIR`, ni build de frontend que copiar al servidor.

## Consumo y reparto de fotovoltaica

El consumo de la casa se calcula con el balance de energía, `producción + red - batería`, que es la misma fórmula que Home Assistant expone como `sensor.consumo_total_vivienda`. Las pinzas de potencia no cubren la casa entera, así que quedan como detalle por circuito y como respaldo cuando no hay fuente de red configurada en la topología.

La serie de 5 minutos y el KPI del día en curso salen del `sum` de las estadísticas de 5 minutos de los contadores de Home Assistant (consumo, red importada y red exportada), el mismo criterio que los días ya consolidados. Si la fuente de red está configurada pero caída, el consumo se sirve como no disponible en vez de con una cifra menor que la real.

El reparto de fotovoltaica del cargador usa ese mismo balance, descontando siempre la energía del cargador, que forma parte del consumo de la casa. Sin balance utilizable, la fotovoltaica del tramo se guarda sin dato en lugar de estimarse sobre las pinzas.

## Desarrollo

```bash
go mod tidy
go build ./cmd/server      # deja el binario ./server
HELIOS_DATA_DIR=/tmp/helios-data ./server
```

La UI templ necesita los artefactos generados (`templ generate` y el CSS de Tailwind). Están commiteados y `make build` en la raíz del repositorio los regenera antes de compilar, dejando el binario en `/tmp/opencode/helios-go-templ` salvo que se pase `BIN=`.

Gates antes de dar un cambio por bueno:

```bash
go test -race ./...
go vet ./...
```

## Configuración

Variables de entorno que lee `internal/config`:

| Variable | Para qué |
|---|---|
| `PORT` | Puerto HTTP, 8080 por defecto. |
| `HELIOS_DATA_DIR` | Directorio de datos. Acepta `DATA_DIR` como compatibilidad y cae a `~/.local/share/helios`. |
| `HELIOS_DB_PATH` | Ruta de la base. Por defecto `<data dir>/helios.db`. |
| `HAOS_URL` | URL de Home Assistant. Si falta, `http://192.168.10.244:8123`. |
| `HAOS_TOKEN` | Token de larga duración. Sin él no hay cliente de HA y la API solar queda deshabilitada. |
| `HAOS_ALLOW_INSECURE` | `1`, `true`, `yes` u `on` permiten HA en claro fuera de loopback. |
| `TRUSTED_PROXIES` | Lista separada por comas de IPs o CIDR de los que se acepta `X-Forwarded-For`. |
| `PRICE_IMPORT_EUR`, `PRICE_EXPORT_EUR`, `CO2_KG_PER_KWH` | Precios y factor de CO2 por defecto. Los valores editados en Ajustes, guardados en KV, ganan sobre estas variables. |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` | Web Push. |
| `SESSION_SECRET` | Secreto de las sesiones. |
| `AUTH_USER`, `AUTH_PASS` | Crean el admin inicial al arrancar si el usuario no existe. |

La URL de HA se valida al construir el cliente: con `http` o `ws` fuera de `localhost` o loopback se rechaza el arranque salvo que `HAOS_ALLOW_INSECURE` esté activo.

## Subcomando `recompute-charger-pv`

```bash
go run ./cmd/server recompute-charger-pv -days 9
```

Con el binario ya compilado es el mismo subcomando, `./server recompute-charger-pv -days 9`. La bandera `-h` imprime las opciones y termina con el error de ayuda de Go (`flag: help requested`) sin llegar a abrir la base.

Una sola pasada que reescribe `pv_kwh` de `daily_charger` en los días ya cerrados con el reparto del balance. Sirve para deshacer los días que quedaron guardados con el reparto de las pinzas. No se ejecuta en el arranque ni en ningún cron, se lanza a mano.

- `-days N` es la ventana hacia atrás. Por defecto 9 días, la misma que rellena el backfill del cargador. Hoy nunca entra, porque lo sigue acumulando el tick de cada minuto. Un valor de 0 o negativo usa la ventana por defecto.
- Solo escribe `pv_kwh`. No toca la energía acumulada del día ni crea filas.
- Es idempotente. Si el reparto nuevo difiere del guardado en 0,0005 kWh o menos, no reescribe y lo cuenta como día sin cambios.
- Deja intactos los días sin balance utilizable y los lista con el motivo: sin energía acumulada que repartir, sin balance de consumo ese día, contador de consumo que retrocede o curvas que no dan reparto.
- Necesita `HAOS_TOKEN` y conexión con Home Assistant, a la que espera 30 s. Exige además el cargador activo en los ajustes.
- Aplica la misma regla de transporte que el servidor. Con HA en claro fuera de loopback hace falta `HAOS_ALLOW_INSECURE=1`.

Imprime un resumen con la ventana recorrida y los contadores de días:

```text
reparto FV del cargador: <desde>..<hasta>
  dias recorridos: <n>
  dias actualizados: <n>
  dias sin cambios: <n>
  dias omitidos: <n>
    - <fecha>: <motivo>
```

Al lanzarlo desde una unidad transitoria de `systemd-run`, hay que pasar `--property=Environment=HAOS_ALLOW_INSECURE=1`, porque la unidad no hereda los drop-ins del servicio.

## Estructura

- `cmd/server/main.go`: entrypoint, wiring y bucles de fondo (alertas, consolidación diaria, acumulación de cargadores, cola push, backfill).
- `cmd/server/recompute.go`: subcomando `recompute-charger-pv`.
- `internal/api`: rutas `/api/*`, `/health` y `/api/version`.
- `internal/auth`: login, sesiones en cookie y rate limit.
- `internal/config`: variables de entorno.
- `internal/db`: apertura de SQLite y migraciones.
- `internal/ha`: cliente WebSocket de Home Assistant.
- `internal/solar`: balance, consumo en vivo, series de 5 minutos, KPIs, backfill y consolidación diaria.
- `internal/install`: topología de la instalación (inversores, batería, contadores).
- `internal/extensions`: cargadores, BYD y reparto de fotovoltaica.
- `internal/alerts`, `internal/push`, `internal/sse`, `internal/update`, `internal/version`: alertas, Web Push, hub SSE, auto-actualización y versión.
- `internal/web`: UI templ + Datastar y assets embebidos.

El detalle de la arquitectura, los contratos y el despliegue está en `ARCHITECTURE.md` y `STACK.md` en la raíz del repositorio.
