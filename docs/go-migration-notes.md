# Notas del port a Go — guía para futuros ports (Deltos)

Notas escritas al terminar el port de Helios a Go (v1.0.0, octubre 2026).
Cuando se migre Deltos, este documento es el mapa de lo que ya se pisó: los
bugs que salieron, las decisiones que funcionaron y el proceso que evitó
romper producción.

## El port de referencia

- `server-go/` reemplaza a `server/` (Node/Hono) con el MISMO contrato HTTP/SSE.
- El frontend React no cambió una línea: el contrato se mantuvo campo a campo.
- Producción validada en paralelo antes del swap (instancia con copia de BD +
  cookie inyectada) y luego corte con rollback listo.
- 100+ tests, `-race`, `vet`, `staticcheck` limpios.

## Decisiones que funcionaron

| Decisión | Motivo |
|---|---|
| **modernc.org/sqlite (pure Go)** | El binario con CGO compilado en CachyOS lleva ISA level znver4 y el loader de Debian lo rechaza ("CPU ISA level is lower than required"). Pure Go = binario 100% estático, cross-compile trivial desde CI. DSN: `file:path?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` |
| **net/http + chi** | El contrato es simple; chi para grupos y middleware. Sin framework pesado. |
| **Tipos tolerantes con `json.RawMessage`** | Los campos que HA/ terceros mandan en dos formatos (epoch vs ISO, número vs string) rompen el unmarshal y descartan el MENSAJE ENTERO. Ver más abajo. |
| **Interfaz por paquete** (`HAReader`, `HA`) | Permite fakes en tests sin WebSocket real. |
| **Validación en paralelo** | Instancia en puerto de prueba con copia de BD + cookie inyectada por SQL, antes de tocar el servicio en vivo. |
| **bcrypt directamente compatible** | Los hashes de `bcryptjs` (Node) los valida `golang.org/x/crypto/bcrypt` sin migración de usuarios. |

## Los 5 bugs reales del port de Helios (todos con test de regresión)

1. **Panic por escritura concurrente al WebSocket.** gorilla/websocket solo
   admite UN escritor. Con varios productores (ticks, backfills, handlers
   HTTP) → `panic: concurrent write to websocket connection` y el proceso
   muere. Fix: `writeMu sync.Mutex` serializando WriteJSON/WriteControl.
   Reproducido con 50 llamadas concurrentes (3 panics) + `-race`.
2. **Deadlock en el readLoop.** Callbacks (`OnConnected` → subscribe) que
   hacen `Call` desde el propio readLoop esperan un resultado que solo el
   readLoop procesa. Fix: callbacks y resuscripciones en goroutine.
3. **Keep-alive.** HAOS cierra la conexión a los 60 s sin pings. Fix: ping
   cada 30 s + read deadline 90 s.
4. **Campos con dos formatos.** `lu` llega como epoch numérico (1792692300.36)
   o ISO; `s` como string o número (mock de la demo). `json.RawMessage` +
   normalizadores (`luToString`, `rawToString`). El unmarshal fallido
   DESCARTABA EL MENSAJE ENTERO → panel a cero.
5. **Atributos parciales en diffs.** `event.c[id]["+"]` trae solo los
   atributos que cambiaron; reemplazar el mapa completo perdía
   `gridDirection`/`stationName`/`inverterOnline`. Fix: MERGE (copia +
   overlay), no replace.

Además: **orden determinista por índice** en los mapeos que en Node dependían
de `Object.keys` (los maps de Go itran aleatorios; el bug se manifestó como
glitch offsets aplicados al inversor equivocado).

## Contrato JSON: undefined vs null

El server Node **omitía** las claves `undefined` (`JSON.stringify`). Go
serializa punteros nil como `null`. El frontend comprueba
`live.switchOn === undefined` → con `null` ese check es `false` y el botón
power quedaba habilitado sin datos. Fix: `omitempty` en TODOS los
opcionales del payload. Regla para Deltos: auditar qué claves comprueba el
front con `=== undefined` y usar `omitempty` en ellas.

## Deploy (systemd, sin Docker)

- `EnvironmentFile=` **gana** sobre `Environment=`: para override de puerto en
  una prueba, usar un segundo EnvironmentFile.
- Sustituir el binario con el servicio activo → "Text file busy" (el `scp`
  falla silencioso y el servicio sigue con el binario viejo). Parar, esperar
  la liberación, copiar, verificar md5, arrancar. Y comparar SIEMPRE el mismo
  tipo de hash (el falso ERROR de Helios era comparar md5 remoto con sha256
  local).
- Tras arrancar, verificar en el log "listening on :PUERTO" antes de dar por
  bueno un smoke test (las instancias muertas dejan el puerto ocupado y las
  peticiones van a la instancia anterior).
- El test e2e de Playwright arranca el binario Go con el MISMO contrato de
  variables de entorno que el Node (`PORT`, `DATA_DIR`, `STATIC_DIR`,
  `HAOS_TOKEN`, `AUTH_*`).

## CI (fallos del release, por este orden)

1. `app/` no tiene script `typecheck` (vivía en `server/`): usar los scripts
   reales de cada paquete.
2. `upload-artifact` NO expande variables de shell en `path`: usar globs.
3. `helios_*` no matchea `helios-web_*`: listar ambos patrones en checksums y
   `files`.

## Checklist específico para el port de Deltos

Estado de partida (22-Oct-2026): backend 8.862 líneas TS, 119 rutas
(36 GET / 43 POST / 14 PUT / 19 DELETE / 7 PATCH), 29 tablas, 5.583 líneas de
tests, frontend 21.5K líneas con 14 dependencias.

- [ ] Driver: `modernc.org/sqlite` (sin CGO, estático) + WAL + busy_timeout.
- [ ] 29 tablas: portar el esquema completo + `PRAGMA user_version` por
      migraciones tolerantes antes de tocar datos.
- [ ] Auth: bcrypt compatible (golang.org/x/crypto), sesiones cookie firmada,
      rate-limit por IP en tabla, audit log.
- [ ] SSE: throttle 1 s + heartbeat 25 s + límite de clientes (patrón
      `internal/sse/hub.go` de Helios).
- [ ] Web Push: `SherClockholmes/webpush-go` (probado en Helios: i18n,
      quiet hours, cola, borrado de suscripciones 404/410, retries con
      backoff).
- [ ] WebSocket a HAOS NO aplica (Deltos no habla con HA); el patrón de
      cliente tolerante vive en `internal/ha/client.go` de Helios por si
      hace falta.
- [ ] 119 rutas: portar por bloques (auth → CRUD core → SSE → push →
      gamification/NLP al final) y validar cada bloque con test de contrato
      del JSON.
- [ ] Los tests de Node (5.5K líneas) NO se portan 1:1: se portan las
      aserciones de contrato y se complementan con tests de integración del
      binario.
- [ ] Validar en paralelo con copia de la BD real antes del swap; rollback
      documentado (el binario Node queda en el host).
- [ ] Tras el swap: vigilar el mismo trío que en Helios (unmarshal descartado
      por campo con dos formatos, null vs undefined en JSON, escritura
      concurrente al WS si lo hay).

## Orden en el que salieron los bugs (aprendizaje)

No fueron aleatorios: se concentraron en las fronteras (parsing de JSON
externo), en la concurrencia (goroutine del readLoop + escrituras) y en el
contracto JSON con el front (`undefined` vs `null`). Las herramientas
(-race, staticcheck) no vieron NINGUNO de los 5: los encontraron los tests
con fixtures reales capturados de producción y la validación en paralelo.
