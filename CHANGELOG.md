# Changelog

Todos los cambios notables de Helios se documentan en este fichero.

El formato se basa en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/),
y este proyecto se adhiere a [Versionado Semántico](https://semver.org/lang/es/).

## [Unreleased]

## [0.8.9] - 2026-08-15

### Changed

- **Tema en móvil**: el header móvil usa ahora un único botón sol/luna que
  alterna claro/oscuro, en lugar del selector de tres iconos (auto/claro/oscuro)
  cuyas etiquetas no caben en pantallas pequeñas. La topbar de escritorio
  mantiene el selector completo con etiquetas. (#95)

## [0.8.8] - 2026-08-14

### Added

- **Marco de extensiones**: módulos opcionales por instalación con interruptor
  maestro y por módulo en la misma barra de Ajustes; cada extensión es editable
  (entidades, unidades, sinónimos de estado) tras su icono de config. (#94)
- **Cargador de coche**: nueva vista con estado en vivo (cargando/enchufado/en
  reposo, potencia, sesión, total, temperatura), curva del día e histórico por
  semanas/meses/años con atribución solar (de solar vs red y batería). (#94)
- Helios almacena su propio histórico del cargador en SQLite: el recorder de HA
  solo conserva ~10 días y muchos cargadores locales no exponen estadísticas.
- Cabeceras de caché HTTP: assets inmutables y shell siempre revalidado.

### Fixed

- Corrección de unidades del cargador: divisor de energía configurable
  (contadores en centésimas de kWh) y potencia en W mal etiquetada como kW.
- Atribución solar por curvas reales durante las ventanas de carga (el balance
  diario neto daba 0% solar en cargas de mediodía de días importadores), con
  flag para cargadores en circuito aparte no medido por los medidores de casa.
- La semana del histórico del cargador incluye hoy; el deep-link a /cargador ya
  no redirige al inicio; gráfica con un único eje.

## [0.8.7] - 2026-08-14
## [0.8.7] - 2026-08-14

### Added

- **Pestañas por inversor desde la topología**: vuelven las vistas individuales
  por inversor (eliminadas en #70) pero sin nombres fijos: se generan desde la
  topología (`install.inverters[].name`). (#91)
- **Etiqueta singular/plural en la navegación**: el menú muestra "Inversor" con
  un inversor e "Inversores" con dos o más (sidebar, raíl, título y navegación
  inferior). (#91)

### Fixed

- **Detección por botón de la pestaña comparativa**: el render de las pestañas
  usaba la pestaña activa para decidir si una tecla era "compare", un bug latente
  que afloraba al reintroducir las pestañas individuales. (#91)

## [0.8.6] - 2026-08-13

### Changed

- **Inversores: donut por defecto**: el "Reparto de hoy" de la comparativa abre
  en vista donut en vez de barras (el toggle sigue funcionando en la sesión).
  (#84)
- **Versión en el pie**: el footer muestra la versión real de la app en runtime
  ("Helios · Monitor Solar vX - datos locales vía Home Assistant"). (#86)
- **Flujo de energía más estrecho en pantallas anchas** (≥1280px): el diagrama
  de flujo pasa de 4/12 a 3/12 de ancho y la tarjeta "Producción vs consumo"
  de 8/12 a 9/12, ganando espacio para la gráfica. (#87)

### Fixed

- **Guiones dobles (em/en dash)**: eliminados los `—`/`–` que quedaban en
  varios textos visibles (pie, subtítulo de SOC, "estás al día", banner de
  demo). Nuevo sanitizer `check-dashes` en el lint: el build falla si aparece
  un em/en dash en index.html, el manifest PWA o las traducciones i18n. (#86)

## [0.8.5] - 2026-08-13

### Changed

- **Inversores: solo comparativa**: con 2+ inversores se eliminan las vistas
  individuales por inversor y se muestra directamente la comparativa (sin
  botones de pestaña). (#70)

## [0.8.4] - 2026-08-13

### Changed

- **Favicon transparente**: ahora es un SVG + PNG con canal alpha real, sin
  fondo de color. Sustituye a los dos `.ico` condicionados por el tema que el
  navegador seleccionaba mal (cuadradito negro en tema claro y viceversa).
  El motivo se escaló (1.49→1.65) para llenar el lienzo tocando los bordes. (#76)
- **Logo de la app**: el icono de marca del shell usa el gradiente ámbar de
  marca (igual que el favicon) en vez de `currentColor` (gris/blanco). (#76)
- **Icono de batería por estado**: nuevo helper `batteryIcon(soc, charging)` —
  cargando → `BatteryCharging`, ≥80% → `BatteryFull`, >30% → `BatteryMedium`,
  >20% → `BatteryLow`, ≤20% → `BatteryWarning`. Aplicado al nodo del diagrama
  de flujo, al strip en vivo y a la KPI del dashboard. (#76)

### Fixed

- La tabla accesible de la gráfica del día mostraba su `<caption>`
  ("Producción y consumo por horas") visible en desktop: los caption se
  posicionan fuera de la caja de la tabla, así que el recorte `sr-only` de la
  tabla no los ocultaba. Ahora `sr-only` se aplica al propio caption. (#76)

## [0.8.1] - 2026-08-12

### Fixed

- **Auto-update apply desde la app (issue #69)**: el botón "Actualizar ahora"
  devolvía 500 porque el servicio va sandboxeado (`ProtectSystem=full` +
  `NoNewPrivileges`) y el `execFile` del script heredaba el sandbox. Ahora el
  endpoint escribe un flag en el dir de datos y un unit systemd
  (`helios-update.path`) lanza `helios-update.service` (root) on-demand; el
  front sondea `/api/version` hasta que el build cambia. Nuevo endpoint público
  `GET /api/version` con el `build` del marker.
- **Cache-buster en `checksums.txt`** del script de actualización: la CDN
  servía una copia vieja justo tras publicar → fallaba la verificación sha256.
- **Workflow de release**: `checksums.txt` ahora se genera en un job separado
  que combina ambos arch (antes cada job de la matriz pisaba el del otro y la
  release quedaba con un solo arch).

## [0.8.0] - 2026-08-12

### Changed

- **Iconos lucide para producción, casa y diagrama de flujo (issue #68)**:
  las placas se muestran con el icono `SolarPanel` (antes `Sun`), ámbar
  mientras la instalación produce y gris cuando no (noche/offline). La casa
  del diagrama de flujo pasa a usar el icono `House` de lucide (se elimina el
  SVG dibujado a mano) y su color refleja la fuente dominante que recibe:
  verde si llega más de la batería, roja si más de la red, ámbar si más de la
  fotovoltaica y neutro cuando no hay consumo. El rótulo del nodo de
  fotovoltaica sube por encima del nodo para no tapar los flujos, el diagrama
  se desplaza para aprovechar el lienzo y el cable de fotovoltaica se alarga
  apurando los márgenes superior e inferior.

## [0.7.9] - 2026-08-12

### Added

- **Pull-to-refresh móvil (issue #28)**: al tirar hacia abajo desde el top de
  la página se recarga la vista actual, para recoger un deploy nuevo sin
  cerrar y reabrir la app. Solo actúa cuando `scrollY <= 0` y el gesto es un
  pull vertical desde arriba (umbral 70 px); no interfiere con el overscroll
  nativo ni con las listas con scroll propio, y el escritorio no se ve
  afectado. Patrón portado de Keynest.

## [0.7.8] - 2026-08-12

### Changed

- **Landing: animación de scroll, sensores de batería, espaciado de gráficas
  y footer (issue #57)**: la sección "Mírala respirar" anima sus estadísticas
  al hacer scroll (respeta `prefers-reduced-motion`); el widget de batería
  queda centrado y gana sensores (temperatura y autonomía estimada); las
  gráficas aprovechan el espacio; corrección de copy ("Hecha para mirarla
  todos los días."); el footer se reduce y su cielo nocturno estrellado pasa
  a la sección CTA "El sol no espera". El final de la página se reordena
  (screens → CTA → principios → FAQ → gracias con corazón rojo → footer, con
  separador entre FAQ y gracias) y la transición luz→noche se acelera con una
  curva de fase no lineal.

## [0.7.7] - 2026-08-12

### Changed

- **Ajustes: editor de topología movido a Conexión y datos (issue #45)**: el
  botón admin "Editar topología" (con su estado configurado/no configurado)
  deja la tarjeta Instalación y pasa a la sección Conexión y datos, en la
  misma fila que "Probar conexión" y junto al listado de entidades HAOS que
  la topología resuelve. La tarjeta Instalación queda centrada en el resumen
  de inversores.

## [0.7.6] - 2026-08-12

### Changed

- **Icono de marca redibujado (issue #59)**: el trazado del sol de Helios se
  rehace a partir de una fuente SVG de Inkscape (casi cuadrada, el motivo
  llena el lienzo). Actualizados los masters `logo.svg`/`logo-square.svg`, el
  logo inline de la app (`BrandLogoIcon`) y regenerados todos los assets
  derivados (favicon claro/oscuro 16+32, `favicon-32.png`, PWA 192/512,
  `apple-touch-icon.png` e iconos de tema). El gradiente ámbar se mantiene.

## [0.7.4] - 2026-08-11

### Changed

- **Ajustes → Acerca de**: los cuatro tiles enlazan ahora a su destino
  (GitHub, la web del proyecto, Ko-fi y el Club Cloudless). El README gana
  los badges de web, demo y Ko-fi (EN+ES). #51

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
