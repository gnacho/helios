# Changelog

Todos los cambios notables de Helios se documentan en este fichero.

El formato se basa en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/),
y este proyecto se adhiere a [Versionado Semántico](https://semver.org/lang/es/).

## [1.2.0] - 2026-10-28

### Added

- **La app abre sin red.** El service worker guarda la última página visitada y
  los estáticos, así que con el móvil sin cobertura se ve el último estado
  conocido en lugar de la pantalla de error del navegador. Los datos en vivo
  (`/api/` y `/ui/`) y el inicio de sesión nunca se cachean, y una página que no
  se había abierto muestra un aviso de "sin conexión" con botón de reintento.

- **`helios-update.sh --check`.** Dice si hay una release más nueva sin
  descargar ni instalar nada: `al dia: 1.2.0` o
  `disponible: 1.3.0 (instalada: 1.2.0)`. No toca el marcador ni el estado del
  actualizador.

### Security

- **Cierre de la auditoría post-migración (29 hallazgos).** Se corrigieron dos
  fallos altos, quince medios y doce bajos detectados tras el port a Go. Los dos
  altos permitían a cualquier usuario autenticado guardar una clave de inversor
  que acababa emitida como expresión Datastar, y publicar releases con acciones
  mutables y permiso de escritura sobre todo el repositorio.

- **Claves y configuración validadas.** La clave del inversor queda limitada a
  entre 1 y 20 caracteres `[a-z0-9-]`, se valida antes de guardar y se sanea al
  normalizar valores ya almacenados. Escribir la configuración global, listar
  usuarios y leer la auditoría exige ahora rol de administrador.

- **Sesiones que mueren cuando deben.** Borrar un usuario elimina sus sesiones en
  la misma transacción, restablecer una contraseña las revoca y una sesión cuyo
  usuario ya no existe se rechaza. Un administrador tampoco puede quitarse el rol
  a sí mismo.

- **Entrada y conexiones acotadas.** El cuerpo de las peticiones se limita a 1 MiB
  (12 MiB en subidas de imagen) y responde 413 si se declara o se envía de más.
  El servidor fija tiempos de cabecera, lectura e inactividad y un tamaño máximo
  de cabeceras. El limitador de acceso solo acepta `X-Forwarded-For` de los
  proxies configurados, purga intentos antiguos y ya no se puede esquivar rotando
  la dirección declarada.

- **Navegador más estricto.** El redirect posterior al acceso rechaza caracteres
  de control, separadores codificados, barras invertidas, userinfo y URLs
  absolutas. Las rutas de la interfaz envían `Cache-Control: no-store`, el service
  worker deja de cachear navegaciones y borra la caché anterior al activarse. Las
  mutaciones del navegador rechazan un `Origin` o `Referer` de otro host. Todas
  las respuestas llevan `nosniff`, `X-Frame-Options`, `Referrer-Policy` y
  `Permissions-Policy`. Los avatares y las imágenes de cargador se decodifican,
  se limitan por número de píxeles, se verifican contra el formato declarado y se
  recodifican antes de guardarse.

- **Integraciones salientes sin SSRF ciego.** Las suscripciones push solo admiten
  HTTPS sin userinfo y rechazan loopback, rangos privados, link-local, CGNAT y
  reservados, validan la IP durante el dial, no siguen redirecciones, tienen
  tiempo límite y envían con un máximo de cuatro en paralelo. Home Assistant exige
  TLS fuera de loopback salvo opt-in explícito de laboratorio, y el WebSocket, las
  respuestas REST y el histórico quedan acotados a 1 MiB, 8 MiB y 50000 filas.

- **Actualizador y release endurecidos.** El progreso vive en un directorio
  propiedad de root fuera del path escribible por el servicio y se publica por
  enlace, sin seguir symlinks. El binario y los backups quedan de root con modos
  restringidos, las descargas solo por HTTPS con redirecciones, tiempo y tamaño
  acotados, y solo se actualiza a una versión estrictamente mayor. Si el health
  check falla se revierten binario y marcador juntos. Las acciones del workflow se
  fijan por SHA completo, `contents: write` se queda solo en el job de release, el
  tag llega por entorno con validación semver y Tailwind y Gitleaks se verifican
  por SHA-256.

- **Build y logs.** `web-check`, la release y la landing fallan si hay artefactos
  generados modificados o sin rastrear. `web-check` descarga el binario de
  Tailwind de la arquitectura del runner (x64 o arm64), así que la matriz de
  release compila en ambas. Los logs sanean rutas y valores, redactan
  los endpoints push hasta esquema y host, y ocultan credenciales, query y
  fragmento de la URL de Home Assistant.

- **Permisos en disco.** El directorio de datos se crea con 0700 y la base, el WAL
  y el SHM quedan con 0600, incluidos los ficheros que ya existían.

- **Nota de actualización.** Tras actualizar conviene revisar `TRUSTED_PROXIES`: si
  está vacío, las cabeceras de proxy se ignoran. El proxy inverso debe conservar
  el `Host` original, y `HAOS_ALLOW_INSECURE` es solo para laboratorio.

## [1.1.8] - 2026-10-25

### Added

- **El histórico anual deja de ser un adorno.** El mapa de calor permite elegir
  entre producción, consumo y autoconsumo (naranja, azul y verde), y cada
  métrica escala sus niveles sobre su propio máximo. Al pasar el ratón por un
  día se ve la fecha y su cifra; al pulsarlo se abre una ficha con el reparto
  solar directa, batería y red de ese día y un botón para ir a él. La tarjeta
  se adapta al ancho disponible y la leyenda usa cuadros del tamaño exacto de
  las celdas.
- **Todas las gráficas responden al ratón.** El dashboard, el día de cada
  inversor, la comparativa, la potencia y el estado de carga de la batería, la
  semana de balance, la curva y el histórico del cargador, las dos vistas del
  histórico y los últimos catorce días muestran una línea de cursor y un
  bocadillo con la hora y el valor de cada serie. En los donuts de origen de la
  energía el sector bajo el ratón se amplía y su dato sustituye al total en el
  centro.
- **Las pestañas se arrastran en el móvil.** En el cargador (de pared y
  portátil) y en inversores (Solis, Fox y comparativa) la tarjeta se coge con el
  dedo: la vecina asoma mientras se desliza, el gesto encaja si pasa de un 22 %
  del ancho y ofrece resistencia en los extremos. No hay recarga ni consulta al
  servidor, porque las dos caras ya vienen pintadas.
- **Las alertas de la campana se pueden dar por vistas.** Cada aviso lleva un
  check que lo quita de la lista sin recargar, y el punto de la campana
  desaparece cuando no queda ninguno. Se guarda por usuario y un aviso que
  vuelve a ocurrir se muestra otra vez.

### Changed

- **Cambiar de pestaña o de periodo ya no recarga la página.** Las pestañas del
  cargador y de los inversores, el selector Semana/Mes/Año, las flechas, el chip
  "Hoy" y el interruptor de normalizar la comparativa piden solo el fragmento
  que cambia y lo reemplazan en su sitio, con las gráficas reanimándose.
- **La batería cabe en una tarjeta.** El estado de carga con sus datos y el
  reparto de origen de la energía comparten una sola tarjeta en una fila, con el
  arco y el anillo al mismo tamaño, los textos juntos y el aire repartido por
  igual a los dos lados del separador.
- **El cargador solo enseña lo que el aparato publica.** Si el dispositivo no
  expone potencia, temperatura o energía total, esas filas desaparecen en vez de
  mostrar ceros o "—". El cargador portátil (cuya integración en la nube lleva
  días caída) muestra únicamente su sesión y un aviso de que aún no hay
  histórico, en lugar de una gráfica plana.
- **Las fotos del cargador se guardan en JPEG cuando no tienen transparencia.**
  Pasan de 380 KB a unos 12 KB, se sirven con ETag y respuesta 304, y aparecen
  con un fundido de 0,45 s que tapa el pintado por franjas. Además se suben
  desde un único icono: al elegir el fichero se envía solo y la página vuelve a
  la misma pestaña.
- **Fuera la SPA: un solo binario.** La app React (`app/`) se ha retirado del
  repositorio. El binario Go sirve ya la API y la interfaz templ, y los
  estáticos raíz (favicon, iconos, `manifest.webmanifest`, `sw.js` y la imagen
  de "sin datos") van embebidos en el ejecutable: desaparecen `public/`, la
  variable `STATIC_DIR` y el flag `HELIOS_UI`, y no queda ningún paso de `npm`
  ni bundle de cliente que desplegar.
- **CI más corta.** Se eliminan los jobs de frontend y de end-to-end (necesitaban
  Node y el build de Vite). Quedan las pruebas del servidor, el build de la
  landing y el escaneo de secretos.
- **La release publica un único artefacto.** El workflow de release deja de
  empaquetar el frontend: sube el binario por arquitectura y los checksums.
  `deploy/helios-update.sh` descarga y verifica solo el binario, y mantiene el
  backup y el rollback automático si el servicio no responde.

### Fixed

- El histórico del cargador mostraba una gráfica vacía cuando el aparato no
  había cargado nada, porque el criterio de "hay datos" incluía la producción
  solar de la casa. Ahora dice que todavía no hay histórico.
- La subida de la foto del cargador volvía siempre al primer cargador: ahora
  conserva la pestaña desde la que se subió.

### Removed

- La app React, sus pruebas end-to-end, `app/public/` y la caché de `npm` en CI.

## [1.1.7] - 2026-10-24

### Changed

- **La línea de autoconsumo del histórico se dibuja.** En las gráficas de
  semana, mes y año el trazo discontinuo verde se traza de izquierda a derecha
  con la misma animación que las barras, y los puntos aparecen a su paso.
  Respeta el interruptor de animaciones y se reinicia al cambiar de periodo.

## [1.1.6] - 2026-10-24

### Changed

- **Cambiar de sección ya no recarga la página.** Los enlaces del menú piden
  solo el contenido de la sección destino y se parchean el contenido, el
  encabezado y el estado del menú; el armazón (logo, barra lateral, cabecera) y
  la conexión en vivo se mantienen. En acceso remoto cada recarga costaba una
  ida y vuelta completa (114 ms de TTFB medidos al dominio) y Firefox dejaba el
  viewport vacío durante la espera: ese era el parpadeo al cambiar de menú.
  Atrás y adelante del navegador siguen funcionando.

## [1.1.5] - 2026-10-24

### Fixed

- **La página se quedaba vacía un instante al cambiar de menú (Firefox).** Al
  navegar, Firefox vacía el viewport mientras el documento nuevo espera a las
  hojas de estilo que bloquean el render, y entre ellas estaba la de Google
  Fonts: otro dominio, con su ida y vuelta en cada cambio de página. Medido con
  capturas sin pérdida, había un fotograma con la página entera vacía.
- **Las fuentes ahora se sirven desde la propia app** (`/assets/fonts`). Inter y
  Space Grotesk son variables, así que un único fichero por familia cubre todos
  los pesos (76 KB en total) y de paso desaparece la petición a Google.

## [1.1.4] - 2026-10-24

### Fixed

- **El histórico diario dejó de crecer.** El servidor Go no tenía ningún proceso
  que consolidara el día: `daily` solo se escribía en el backfill inicial, que
  únicamente corre con la tabla vacía. El histórico se quedó parado en la fecha
  de ese backfill y días como el 22 y el 23 de octubre salían como "sin datos
  para este periodo". Ahora, al arrancar y cada 30 minutos, se rellenan los días
  que falten desde el último consolidado (idempotente y sin escribir nunca el
  día en curso, que sigue siendo parcial).

## [1.1.3] - 2026-10-24

### Fixed

- **Fogonazo al cambiar de menú (visto en Firefox, tema oscuro).** Firefox pinta
  el documento nuevo antes de aplicar la hoja de estilos externa, y el color de
  fondo solo estaba en esa hoja: se veía un instante en blanco. Ahora el fondo y
  el `color-scheme` van en un `<style>` en línea en el `head`, con
  `prefers-color-scheme` para el tema automático. Medido retrasando la hoja 400
  ms: el fotograma del primer pintado pasa de brillo 255 (blanco) a 13 (fondo
  oscuro) en tema oscuro.

## [1.1.2] - 2026-10-24

### Fixed

- **Parpadeo de las zonas comunes al cambiar de menú en escritorio.** La
  transición de navegación entre páginas capturaba la página entera, así que el
  sidebar (logo, menú, ajustes) se fundía junto con el contenido aunque no
  cambiara. Ahora esa transición solo se aplica en móvil, donde está diseñada,
  y en escritorio el único cambio es la entrada del ítem activo.

## [1.1.1] - 2026-10-24

Auditoría de la 1.1 (staticcheck, govulncheck, `go test -race` y revisión de las
fronteras de entrada).

### Fixed

- **La interfaz no se actualizaba sola.** El hub de SSE salía del broadcast
  cuando no había clientes de la API antigua, y la UI nueva suscribe en otra
  pool: el stream solo entregaba el estado inicial. Además esa segunda pool se
  recorría fuera del mutex, con riesgo de `fatal error: concurrent map
  iteration and map write`.
- **El límite de 8 MB de la foto del cargador no existía.** Se instalaba
  después de leer el formulario, que ya había consumido la petición entera.
  Ahora el tope va antes y además se rechazan las imágenes con más de 40
  megapíxeles antes de decodificarlas (un PNG de 300 bytes podía pedir 3,6 GB).
- Tres vulnerabilidades en `golang.org/x/image` (decodificación WebP/VP8L)
  corregidas subiendo a v0.47.0.
- El caché de series podía quedarse bloqueado para siempre si una consulta
  entraba en pánico: la liberación del single-flight no estaba en un `defer`.
- Un administrador podía cambiarse el rol o borrar su propia cuenta: el guard
  de "no sobre uno mismo" comparaba el nombre de usuario con el id del
  objetivo. De paso, el `user_id` de esas entradas de auditoría ya es el id.
- Precios con `NaN`/`Inf` pasaban la validación de rango.
- Redirección abierta por barra invertida (`/\host`) en los endpoints sin
  sesión.

### Changed

- Limpieza del código muerto que marcaba `staticcheck`.

## [1.1.0] - 2026-10-24

### Added

- **Interfaz nueva servida desde el propio backend (templ + Datastar).** Las
  seis pantallas (panel, inversores, batería, histórico, cargador y ajustes)
  se renderizan en el servidor, sin JavaScript de aplicación: solo el runtime
  de Datastar para el tiempo real, un helper estático de gestos (deslizar
  pestañas) y el service worker. La app React sigue en `app/` y se puede
  arrancar con `HELIOS_UI=react`, pero producción usa `HELIOS_UI=templ`.
- **Preferencias de apariencia** en Ajustes: tres estados de tema (sistema,
  claro, oscuro), acentos, densidad y un interruptor de animaciones, guardados
  por dispositivo en cookie.
- **Precios de la energía, nombre y ubicación de la instalación** en Ajustes,
  guardados en la base de datos. Con ubicación configurada, las horas de sol
  se calculan en local (SunCalc portado a Go) en vez de depender de Home
  Assistant, y el autocompletado de municipios funciona sin conexión.
- **Flujo de energía rediseñado** en el panel: puntos de compra/venta a red,
  SOC en el nodo de batería, colores semánticos y la casa como fuente
  dominante, con tests propios.
- **PWA**: el layout enlaza el manifest y registra el service worker, así que
  la app se puede instalar y seguir recibiendo notificaciones push.

### Changed

- Las animaciones de las gráficas (barras, áreas, quesitos, arco solar y
  contadores) son CSS puro y se disparan al cargar la página. Se retiraron las
  ligadas al scroll: dejaban las gráficas a medio dibujar cuando ya estaban en
  pantalla, y las barras del histórico se quedaban sin animar al cambiar de
  periodo.
- Las gráficas usan la geometría por tarjeta del diseño original, con las
  mismas alturas y márgenes que la app React.

### Fixed

- **El estado del cargador no se actualizaba.** Las páginas abrían el stream
  de señales con `data-on-load`, atributo que no existe en Datastar (el plugin
  es `data-init`), así que ningún valor en vivo se refrescaba hasta recargar.
  Afectaba al panel, batería, inversores y cargador.
- **El botón de encender/apagar del cargador respondía tarde.** La acción se
  ejecuta en Home Assistant en milisegundos, pero la interfaz esperaba al
  siguiente envío del stream. Ahora la petición espera la confirmación de Home
  Assistant y devuelve el estado nuevo, con aviso inmediato de "Encendiendo…"
  o "Apagando…" mientras tanto.
- Los enlaces `data-attr-*` de Ajustes y Cargador no hacían nada: la sintaxis
  correcta es `data-attr:<atributo>`. `aria-pressed`, `title`, `class` y
  `disabled` no se aplicaban nunca.
- **Fotos del cargador**: las dos tarjetas tienen el mismo alto con cualquier
  foto y se recorta el borde vacío de las imágenes ya subidas (vertical y
  horizontal), en lugar de escalarlas y dejar aire.
- **Cambios de CSS que no se veían**: los estáticos se servían con nombre fijo
  y cinco minutos de caché, así que un cambio tardaba en aparecer. Ahora las
  URLs llevan un hash del contenido y se cachean de forma inmutable.
- El cambio de periodo del histórico reinicia todas las animaciones (antes el
  morph de Datastar reutilizaba los nodos y las barras nuevas salían ya
  crecidas) sin mover el scroll de la página.

## [1.0.0] - 2026-10-22

### Changed

- **El backend está reescrito en Go.** Helios deja de correr sobre Node: ahora
  es un único binario estático de ~12 MB, sin runtime ni `node_modules`, que
  sirve la misma API REST y el mismo SSE. El frontend React no cambia ni una
  línea; el contrato de la API se mantuvo y se verificó campo a campo contra
  Home Assistant antes del corte (SOC de batería, producción por inversor, red,
  energía del cargador, curva del día e histórico).
- **SQLite con driver en Go puro** (`modernc.org/sqlite`), para que el binario
  sea 100% estático y compile para amd64 y arm64 desde CI.
- **Actualizaciones por binario**: `deploy/helios-update.sh` descarga el binario
  y el frontend del release, verifica el sha256 de ambos, sustituye de forma
  atómica y hace *rollback* automático si el servicio no responde tras el
  reinicio.
- El backend Node anterior queda en `server/` como vía de rollback, sin uso en
  producción.

### Added

- Módulo **BYD** portado al backend Go (estado del coche, neumáticos, ubicación,
  programación de carga y acciones: arrancar/parar carga, cierre de ventanas,
  bloqueo, climatización, forzar consulta), con el mismo panel de Ajustes.
- **Caché con single-flight** de las series del día y de las curvas del
  cargador: varias peticiones simultáneas de la misma fecha hacen una sola
  consulta a Home Assistant, y se respeta el TTL del servidor anterior.

### Fixed

- Los estados de las entidades llegaban vacíos cuando Home Assistant enviaba
  `last_updated` como marca de tiempo numérica: el mensaje entero se descartaba
  y el panel mostraba 0 en producción, batería y red.
- Los cambios incrementales de entidades traen solo los atributos que han
  cambiado; al reemplazar el mapa completo se perdían `gridDirection`,
  `stationName` e `inverterOnline`, y el balance de red caía al valor de
  consumo. Ahora se combinan.
- Escrituras concurrentes al WebSocket de Home Assistant: el cliente Go podía
  paniquear y tumbar el proceso cuando coincidían dos consultas. Ahora se
  serializan, con test de regresión.
- El cliente se desconectaba al minuto por falta de *keep-alive*; ahora envía
  pings periódicos y reconecta con backoff.
- `/api/system/info` conserva el contrato que lee la pantalla de Ajustes
  (`node`, `react`, `uptime`, memoria).

## [0.8.28] - 2026-10-22

### Added

- **Desliza entre pestañas en el móvil (#154).** En Inversores y en Cargador
  ya no hace falta acertar en la píldora de arriba: arrastra el dedo a los
  lados y el contenido se desliza con él, con la pestaña siguiente entrando de
  lado mientras arrastras. Al soltar, si has pasado de un cuarto de pantalla se
  cambia; si no, vuelve a su sitio. En los extremos la barra hace tope, así que
  no se sale de rango. El scroll vertical y el gesto de recargar siguen
  funcionando igual.

### Fixed

- **Al volver a la pestaña anterior se ve la tarjeta que viene (#154).** Antes,
  al deslizar de vuelta al primer cargador o inversor, la vista anterior no
  aparecía hasta un segundo gesto; ahora entra de lado junto con el dedo, como
  la ida.

## [0.8.26] - 2026-10-19

### Changed

- **Todo el código pasa a TypeScript (#149).** El servidor ahora está escrito
  en TypeScript estricto y se compila con `tsc` a `dist/` (la app sigue
  arrancando igual, ahora desde el código compilado). Los tests del servidor,
  la landing y el service worker también son TypeScript. Sin cambios
  funcionales para el usuario: la app se ve y comporta igual, con un código
  más robusto detrás.

## [0.8.25] - 2026-10-14

### Added

- **Varios cargadores de coche con pestañas (#146).** La página del cargador
  admite más de un cargador (pared, portátil...) con pestañas para alternar,
  foto propia por cargador con botón para cambiarla, botón de
  encendido/apagado junto al estado y control de la corriente de carga con
  chips de amperaje cuando la entidad configurada lo permite. La energía del
  día, las curvas y el histórico se acumulan por cargador.

### Fixed

- **La curva de carga de hoy se detiene en la hora actual (#145).** Antes se
  dibujaba plana hasta medianoche.
- **Subir la foto del cargador ya no deja la página en blanco (#148).**

## [0.8.18] - 2026-09-19

### Fixed

- **Caducidad deslizante de la sesión (#130).** Antes la sesión caducaba 30 días
  después del login, sin renovación. Ahora, cuando queda menos de la mitad de
  ese plazo, la sesión se extiende y la cookie se re-emite: un usuario activo
  no vuelve a perder la sesión por calendario.

## [0.8.20] - 2026-09-19

### Fixed

- **Caducidad deslizante de la sesión (#130).** La sesión renueva su expiración
  y la cookie al usar la app: un usuario activo no vuelve a perder el login por
  calendario.
- **Relleno de huecos en la curva del día (#124).** Cada inversor puede declarar
  un sensor de respaldo (p. ej. el feed de nube del fabricante): cuando el
  primario no tiene datos para un tramo, la curva se completa con el respaldo
  y se escala al total diario real; la curva se marca como estimada.
- **Hueco de consumo total (#125).** Si todos los medidores de consumo caen a
  la vez, el hueco se rellena con la forma del día anterior, escalada al total
  consolidado de ese día.
- **Cabeceras de caché en la API.** Las respuestas de /api y el shell de la
  app ahora se sirven siempre con no-cache (antes la API podía cachearse por
  heurística del navegador).

### Added

- **Tracker de analítica en la demo pública (#112).** La demo de
  demo.helios.cloudless.club registra visitas en GoatCounter (prefijo /demo).
  Solo existe en el build de la demo: las instalaciones self-hosted no llaman a
  casa.

### Security

- **Dependencias actualizadas (#133).** hono 4.13.8, vitest 4.1.11 y js-yaml
  4.3.2 (transitivo): cierra 6 alertas de Dependabot (1 alta).

## [0.8.21] - 2026-10-03

### Changed

- **Diálogo de actualización más grande (#137).** El diálogo de actualización
  ahora ocupa casi toda la ventana en escritorio, con las notas de la release
  en un área con scroll que crece, cabeceras de secciones en negrita y las
  acciones ancladas abajo. En móvil se mantiene el comportamiento actual.

## [0.8.22] - 2026-10-03

### Fixed

- **Diálogo de actualización ajustado (#139).** El diálogo ya no ocupa casi
  toda la pantalla: unos 800x600 en escritorio con las notas haciendo scroll
  dentro, y la página de fondo ahora se ve claramente difuminada, no solo
  oscurecida.

## [0.8.24] - 2026-10-07

### Changed

- **Scrollable mobile bottom nav (#142).** The mobile bottom nav is now a
  horizontally scrollable row with every section as a direct tab,
  auto-centering the active one. The fixed 4/5-column grid (which depended
  on the charger extension being on) is gone, consistent with the other
  apps in the stack.

## [0.8.23] - 2026-10-03

### Fixed

- **Dependencias con vulnerabilidades conocidas (#141).** brace-expansion (2
  alertas de Dependabot) estaba pineado por overrides a versiones vulnerables;
  ahora va a las parcheadas (1.1.21 / 5.0.12) y nanoid se actualiza por encima
  del aviso GHSA-2v37-7h3g-55p8. npm audit queda limpio; build, lint y los 115
  tests del servidor siguen en verde.

## [Unreleased]

## [0.8.14] - 2026-08-29

### Fixed

- Añade el tracker de Umami a la landing para registrar visitas (#118).

## [0.8.13] - 2026-08-28

### Added

- **Asistente de actualización**: diálogo completo con confirmación, changelog
  de la release, pasos detallados (buscar, descargar, verificar, instalar,
  reiniciar) con barra de progreso y porcentaje. Requiere confirmar la caída
  del servicio antes de aplicar. Recibe el progreso en tiempo real por SSE
  con fallback a polling. (#117)

### Fixed

- **Script de update con progreso fino**: `helios-update.sh` ahora escribe
  `update-progress.json` en cada paso para que el frontend muestre el avance
  detallado en lugar de un spinner genérico. (#117)

## [0.8.12] - 2026-08-28

### Added

- **Histórico ilimitado de curvas**: Helios guarda la curva de cada día en su
  propia base de datos (`day_series`), de modo que las gráficas del histórico
  dejan de depender de la retención (10 días) del recorder de HAOS. Al arrancar
  y en la consolidación nocturna, un backfill idempotente rellena las curvas de
  los días que HAOS aún conserva y, si existe un backup de HAOS previo, también
  los días de esa fuente. Los días sin sensor de potencia de un inversor se
  marcan como curva estimada. (#114)

### Fixed

- **Comprobación de actualizaciones sin rate-limit**: el ribbon y el botón
  "Comprobar actualizaciones" de Ajustes ahora usan el endpoint del servidor
  (`/api/update/status`, con caché de 5 min) en lugar de consultar
  `api.github.com` directamente desde el navegador. Elimina los 403 silenciosos
  por rate-limit en IPs compartidas. (#116)

## [0.8.11] - 2026-08-18

### Added

- **Aviso de batería llena**: nueva alerta push que avisa una vez cuando la
  batería llega al 100 % y se rearma al bajar del 95 % (la misma histéresis
  de 5 puntos que la alerta de batería baja). Severidad normal: respeta las
  horas de silencio y tiene interruptor propio en las preferencias de
  notificaciones de Ajustes. (#104)

## [0.8.10] - 2026-08-15

### Added

- **Deslizamiento entre vistas en móvil**: al cambiar de pestaña con la barra
  inferior, el contenido se desliza en la dirección de la navegación (adelante
  o atrás según el orden del menú). El header y la barra inferior permanecen
  fijos durante la transición. (#93)

### Changed

- **Ajustes en móvil**: el avatar del header (arriba a la derecha) enlaza a
  Ajustes y la barra inferior queda solo con las vistas del dominio; en la
  tablet, el raíl ya no duplica el icono de Ajustes. (#99)

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
