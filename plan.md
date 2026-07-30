# Plan — Dashboard Solar Unificado (Solis + Fox) sobre HAOS

## Contexto
- Inversor 1: Solis, 10 placas × 440 W (4,4 kWp) + batería Soluna 5 kWh
- Inversor 2: Fox, 6 placas × 450 W (2,7 kWp)
- Datos ya unificados en Home Assistant (HAOS), accesibles vía API REST/WebSocket local
- Objetivo: vista web interactiva de solo lectura, muy visual, estilo SolisCloud/FoxCloud
- No exponer HAOS: la web se conecta en local; luego se hará PWA exponiendo solo esa web

## Stage 1 — Iteración de diseño frontend (datos mock) ← AHORA
- Skill: `vibecoding-webapp-swarm` (+ `webapp-building-swarm` si hace falta)
- Entregable: app React con datos simulados realistas:
  - Diagrama de flujo de energía animado (PV → casa/red/batería) estilo SolisCloud
  - Producción por inversor (Solis 4,4 kWp / Fox 2,7 kWp) y total
  - Consumo del hogar, importación/exportación de red
  - Estado batería Soluna: SOC %, potencia carga/descarga
  - Gráficas del día (producción vs consumo), estadísticas (hoy/mes/año)
- Iterar con el usuario sobre el diseño hasta aprobarlo
- Validación: build OK + versión guardada con website_version_manager (preview)

## Stage 2 — Capa de datos HAOS (backend ligero) ✅ HECHO (29-Jul-2026)
- Backend Node 20 + Hono + ws + better-sqlite3 en `server/`
- Conexión HAOS: WebSocket persistente (`subscribe_entities`) con reconexión; REST solo para pruebas
- Endpoints: `/api/solar/live`, `/day`, `/kpis`, `/history`, `/solar/stream` (SSE), `/api/auth/*`, `/api/config`
- Auth: login usuario+pass, cookie de sesión firmada HMAC (SQLite), rate-limit 5 intentos
- Histórico: SQLite propia + backfill desde `recorder/statistics_during_period` (period day, types ["state"])
- Mapeo de entidades en `server/src/config.js` (override por env)
- Decisiones clave:
  - Consumo = suma de los 3 medidores Zigbee (NO `sensor.consumo_total_vivienda`, tiene bug con gridPower con signo)
  - Grid = `Math.abs(currentGridPower)` + `gridDirection` del scraper Solis
  - Serie del día: statistics 5min de potencias; signo batería por pendiente del SoC; grid derivado por balance
- Frontend: provider real (SSE + caché) sustituye al mock; login propio (AuthGate); Ajustes con estado real

## Despliegue ✅ (29-Jul-2026)
- **CT 226 `helios`** en host-a: Debian 13, 1C/512M/4G, IP 192.168.10.226 (reserva DHCP Flint2)
- App en `/opt/helios/` (server + public + data), servicio systemd `helios.service`, puerto 80
- Credenciales app: usuario `demo`, pass en `.env` del CT (generada aleatoria)
- **Expuesta**: `https://helios.example.com` (NPM proxy host id 9 + LE id 12, Force SSL)
- DuckDNS auto-update cada 5 min (cron en el CT)
- **Histórico profundo**: 798 días importados (Fox desde 15-ene-2024, Solis ago-2025, red oct-2025, batería ago/nov-2025, consumo desde 20-nov-2025). Glitches pinza Fox corregidos en código (`FOX_GLITCH_OFFSETS`).
- **Lección clave**: totales diarios de UMs = diff de `sum` (el `state` de la fila diaria es el valor a las 00:00 = 0). Contadores lifetime = diff de `state`.
- **Consumo (30-Jul)**: los 3 medidores solo cubren ~40% de la casa (faltan piscina/calentador/invitados/¿cargador?). Helios usa `max(medidores, balance)` con balance = producción + import − export − carga + descarga (misma cuenta derivada del Energy panel de HA). Balance mensual cierra a ±22 kWh. Realidad: en verano se consume MÁS de lo que se produce (jun: 1095 vs 1039).

## Stage 3 — PWA + despliegue (parcial)
- Manifest e iconos ya existen; falta service worker real (offline cache)

## Pendientes conocidos (30-Jul-2026)
1. **Bug HAOS** `sensor.consumo_total_vivienda` (gridPower con signo tratado como magnitud → 0 de noche e inflado de día). Afecta al Energy panel de HA, no a Helios. Fix de una línea pendiente de OK del usuario.
2. Service worker PWA.
3. Página Batería: valores decorativos hardcodeados (ciclos 312, temp 28 °C real ~44 °C — el scraper no expone temp BMS).
4. Medición por circuitos incompleta (~60% de la casa sin medir) — decisión hardware pospuesta a la casa nueva.

## Notas
- Idioma UI: español
- Diseño: pendiente de respuestas del usuario (tema, prioridad de vistas)

---

## Actualización — decisiones del usuario (fase 2)
- Acceso: se expondrá solo la PWA vía Nginx Proxy Manager (Proxmox, varios servicios)
- Datos en HAOS: Fox vía FoxESS Cloud; Solis vía scraper propio (tiene API cloud como alternativa) → da igual: el backend habla con HAOS, no con los inversores
- Backend: contenedor Docker dedicado (NPM solo proxy inverso)
- Histórico: acumulación propia (SQLite) para heatmap anual
- Entregado: zip del frontend v1 → /mnt/agents/output/helios-dashboard-solar.zip
- Pendiente fase 2: backend Node 20 + Hono + home-assistant-js-websocket + better-sqlite3,
  mismo contenedor sirve dist/ + /api/*, auth propia, NPM delante con SSL
