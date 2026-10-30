# Arquitectura de Helios

Referencia para entender cómo está montado Helios y para replicar el patrón en otras
apps: backend Go, SQLite embebida, pantallas renderizadas en el servidor con templ +
Datastar y todo dentro de un único binario estático. El backend Node que existió hasta
v0.8.x se retiró del árbol en v1.0.0; no queda código suyo.

## 1. Layout

```
server-go/
  cmd/server/main.go     # arranque: configuración, HA, HTTP y jobs
  internal/api/          # API REST/SSE, guardia de origen, límite de cuerpo
  internal/auth/         # login, sesiones en cookie firmada, rate-limit
  internal/config/       # variables de entorno y valores por defecto
  internal/db/           # SQLite (modernc.org/sqlite, puro Go): esquema y consultas
  internal/ha/           # cliente WebSocket de Home Assistant
  internal/solar/        # live, series, KPIs, balance, backfill
  internal/install/      # topología de la instalación (inversores, batería, red)
  internal/extensions/   # cargadores, precios, curvas del día, acumulados
  internal/alerts/       # motor de alertas (flancos, histéresis, resumen diario)
  internal/push/         # Web Push con VAPID
  internal/sse/          # hub de SSE
  internal/update/       # actualización in-app desde GitHub Releases
  internal/health/       # comprobaciones de salud
  internal/version/      # versión compilada
  internal/web/          # UI templ + Datastar y assets embebidos
    assets/root/         # manifest PWA, service worker, iconos
    assets/css|js/       # app.build.css, datastar.js
    locales/             # catálogos i18n (es, en, zh-CN)
deploy/                  # unit de systemd y script de actualización
landing/                 # web de marketing
docs/                    # configuración y operaciones
```

Un solo binario sirve la API, la UI y los estáticos raíz. No hay `public/` ni build de
frontend que copiar: la UI se genera con templ y el CSS con el binario de Tailwind
(`make build`), así que el árbol no necesita Node. El despliegue típico es systemd con
un usuario dedicado y `AmbientCapabilities=CAP_NET_BIND_SERVICE` para escuchar en :80.

## 2. Conexión con Home Assistant

WebSocket persistente a `ws://<haos>/api/websocket`, con el protocolo nativo:

1. El servidor recibe `auth_required` y responde `{type:"auth", access_token}`.
2. Con `auth_ok` suscribe las entidades de la topología:
   `{id, type:"subscribe_entities", entity_ids:[...]}`.
3. El primer evento trae el snapshot completo (`event.a`, con estado y atributos). Los
   siguientes son diffs (`event.c`, con `"+"` y `"-"`).
4. Las llamadas RPC (`recorder/statistics_during_period`, entre otras) llevan un `id`
   incremental y se resuelven por ese id, con timeout.
5. Al caer la conexión reconecta con backoff; al volver, re-suscribe.
6. `auth_invalid` es fatal y no se reintenta.

El cliente guarda el último estado conocido por entidad y el resto de la app lee de
ahí. No hay polling REST.

Las series históricas salen de `recorder/statistics_during_period`:

- `period:"5minute", types:["mean"]` para las curvas de potencia del día.
- `period:"day", types:["state","sum"]` para el histórico diario.
  - En contadores acumulativos, el valor diario es la diferencia de `state`.
  - En utility meters y plantillas diarias, es la diferencia de `sum`. El `state` de la
    fila diaria es el valor a las 00:00, es decir, cero.

## 3. API HTTP

```
POST /login, /logout
GET  /me, /profile, /users, /audit
GET  /api/solar/live          estado en vivo y alertas
GET  /api/solar/day           288 puntos de 5 minutos, con marca de estimado
GET  /api/solar/kpis          hoy desde live, días pasados desde SQLite
GET  /api/solar/history       rango de días desde SQLite
GET  /api/solar/stream        SSE del estado en vivo
GET  /api/charger/history     histórico por cargador
GET  /api/charger/day         curva del día de un cargador
GET  /api/charger/stream      SSE de los cargadores
POST /api/charger/action      encender, apagar o cambiar amperaje
POST /api/byd/action          acciones del vehículo cuando hay integración
GET  /api/config, /api/install, /api/extensions
POST /api/update/apply        lanza la actualización
GET  /api/update/status, /api/update/progress, /api/update/stream
GET  /api/version, /api/system/info, /health
GET|POST|DELETE /api/push/*   suscripción, preferencias y clave pública VAPID
```

- Auth: middleware en todo salvo el login. Cookie `id.hmac`, httpOnly, SameSite=Lax, con
  `Secure` cuando la petición llega por https. El secreto HMAC sale del entorno o se
  genera y se guarda en `kv`, así sobrevive a los reinicios.
- Rate-limit del login: cinco fallos por IP bloquean cinco minutos.
- Guardia de origen en las peticiones mutantes: compara `Origin` y `Referer` con el host
  de la petición y con `X-Forwarded-Host`, de modo que funciona detrás de un proxy. Un
  `Origin: null` solo pasa en las rutas de entrada de sesión o cuando hay una sesión
  válida (el service worker lo envía así).
- SSE: cabeceras `X-Accel-Buffering: no` y `Cache-Control: no-cache`. Sin la primera,
  nginx acumula los eventos y la página parece congelada.

## 4. UI con Datastar

No hay SPA ni capa de datos en el cliente. El servidor manda HTML y parchea fragmentos.

- En vivo: cada página declara `data-init="@get('/ui/stream')"`. El handler abre un
  stream y emite señales con el snapshot (`live`, `chg.*`, `ui.*`) más un ping
  periódico. Los `data-text` y `data-attr` leen esas señales sin JavaScript propio.
- Gráficas: los SVG se generan en Go y se refrescan solos cada minuto con
  `data-on-interval__duration.60s`, que pide el fragmento a `/ui/day-chart`,
  `/ui/chg-curve`, `/ui/bat-charts` o `/ui/inv-charts` y lo sustituye en el sitio.
- Navegación y pestañas: los enlaces hacen `history.pushState` y piden `/ui/nav`,
  `/ui/chg`, `/ui/inv` o `/ui/historico`. El servidor reutiliza el handler de la página,
  extrae el fragmento entre marcadores y lo parchea, sin recargar el documento.
- Formularios: `@post` con señales y autoguardado. El handler responde con el fragmento
  a refrescar o con un 204.
- i18n y formato se resuelven en el servidor, así que el navegador solo recibe texto
  final.

El hover de las gráficas es una capa de bandas transparentes con CSS puro.

## 5. SQLite

Doce tablas: `daily`, `daily_charger`, `day_series`, `kv`, `sessions`, `users`,
`audit_log`, `login_attempts`, `push_subscriptions`, `notification_preferences`,
`notification_queue` y `notification_quiet_hours`.

- `journal_mode = WAL`. Las copias se hacen con la API de backup de SQLite, no con `cp`.
- El esquema se crea con `CREATE TABLE IF NOT EXISTS` y las columnas nuevas se añaden
  con `ALTER TABLE` cuando faltan, comprobando antes `PRAGMA table_info`. No hay
  migraciones numeradas que se editen después.
- El histórico vive aquí y no depende de la retención de Home Assistant. Se rellena con
  un backfill inicial desde las estadísticas de HA y con una consolidación nocturna del
  día anterior.
- `daily_charger` guarda los kWh de cada cargador y su parte solar, y es la fuente de la
  energía que muestra la tarjeta del cargador. No se usa el contador del propio aparato:
  muchos firmwares no lo publican y devuelven cero.
- Litestream replica la base a otro equipo de forma continua.

## 6. Alertas

El motor corre cada minuto y trabaja por flancos, con ticks de confirmación para no
avisar de un parpadeo: inversor caído (solo de día), pinza caída, corte de red y
recuperación, batería baja y batería llena (con histéresis de SOC). El resumen diario
sale al anochecer, a las 21:00 si no hay dato solar.

Las notificaciones usan Web Push con VAPID. Hay preferencias por tipo, horas de silencio
con cola consolidada e i18n en el servidor. Sin claves VAPID la app arranca con el push
desactivado. Cada alerta disparada queda en `audit_log`, lo que permite ajustar umbrales
con historial real.

## 7. Actualización in-app

`helios-update.path` vigila `/opt/helios/data/.update-requested`. Cuando la app escribe
ese fichero, el path descarga el binario de la release, lo sustituye y reinicia. El
rollback entre versiones con esquema distinto no es seguro: conviene copiar la base antes
de actualizar.

## 8. Decisiones de datos (lecciones caras)

1. Un medidor físico siempre es mejor que una plantilla derivada. La plantilla de consumo
   que había tenía un error de signo y daba cero o valores inflados.
2. Algunos inversores publican la potencia de red con signo de fábrica. Hay que quedarse
   con el valor absoluto y guardar la dirección por separado.
3. Para totales diarios de utility meters, diferencia de `sum`, nunca de `state`.
4. Un sensor sin `state_class` no genera estadísticas. Cualquier potencia que se quiera
   graficar necesita `device_class` y `state_class: measurement` en Home Assistant.
5. El consumo real de la casa es el mayor entre los medidores y el balance
   (producción + importación - exportación - carga + descarga). Los circuitos sin medidor
   quedan cubiertos por la derivada.
6. Detrás de un proxy, SSE necesita `X-Accel-Buffering: no`.
7. La UI se verifica contra la API real en un navegador, no solo con curl.

## 9. Despliegue

systemd con usuario dedicado, base y datos en `/opt/helios`. El orden del deploy manual y
los dos errores clásicos (`chmod` antes del `mv` y actualizar `.release-id`) están en
`docs/operations.md`. La configuración por entorno y la topología están en
`docs/configuration.md`.
