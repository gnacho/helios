# Stack tecnológico Helios — referencia para otras apps

## Arquitectura
Un único binario Go sirve la API REST + SSE y las pantallas (HTML renderizado en el
servidor con templ + Datastar). Lee Home Assistant en local (WebSocket) y NPM pone SSL
delante. Sin Docker y sin runtime de JavaScript en el servidor.

## Backend — Go 1.25 (desde v1.0.0), `net/http` + `chi`, binario estático
- **Servidor HTTP**: `net/http` + `chi`, un solo binario estático (`CGO_ENABLED=0`, sin libc), con la UI embebida (`go:embed`)
- **WebSocket HAOS**: cliente propio en `internal/ha` (`subscribe_entities`, `recorder/statistics_during_period`), reconexión con backoff, ping de keep-alive y caché de entidades en memoria
- **SQLite**: `modernc.org/sqlite` (Go puro para que el binario siga estático), WAL, sin ORM y con migraciones incrementales tolerantes
- **Auth**: cookie de sesión HMAC httpOnly 30d, sesiones en SQLite, rate-limit de login en memoria
- **SSE**: hub propio con throttle de 1 s que empuja señales Datastar al navegador
- **UI**: renderizada en el servidor con templ + Datastar, embebida en el binario (sin SPA ni flag de modo)
- **Despliegue**: systemd (`helios-go.service`) con usuario dedicado + `AmbientCapabilities=CAP_NET_BIND_SERVICE` para :80 sin root, y Litestream replicando la BD por SFTP

## Frontend — templ + Datastar (desde v1.1.0)
- **Sin JavaScript de aplicación**: el HTML llega pintado; solo se carga el runtime de Datastar (señales y parches SSE), un helper estático para el gesto de deslizar pestañas y el service worker
- **Estilo**: Tailwind v4 con el binario standalone (sin Node en el build del CSS) y variables CSS para los temas claro/oscuro/sistema, acentos y densidad
- **Animaciones**: CSS puro (keyframes, contadores por señales Datastar). Nada de librerías de animación
- **PWA**: manifest enlazado desde el layout, `sw.js` para notificaciones push y registro del worker en el helper estático
- **Estáticos**: favicon, iconos PWA, `manifest.webmanifest` y `sw.js` viajan en `server-go/internal/web/assets/root/` (embed), así que no hay `STATIC_DIR` ni carpeta `public/` que desplegar

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
