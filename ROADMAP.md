# Roadmap

This is a personal project. The roadmap reflects the direction I have in mind,
not promises or dates. Phases 1 and 2 are done; 3 and 4 are ideas I want to
explore.

| Phase | Focus | Status |
|---|---|---|
| 1 | Live read-only panel (production vs consumption) | Done |
| 2 | Real backend, multi-user, history, battery, alerts, PWA, hardening | Done (~0.6.x) |
| 3 | Configurable inverters and panels (still under HAOS) | Planned |
| 4 | Multi-installation: several HAOS or several Helios via API | Exploring |

## Phase 3 — Configurable hardware, still under HAOS

Today the installation (inverters, panels, battery) and the HAOS sensor mapping
are hardcoded for one setup. Phase 3 makes that configurable so another
installation with a different number or model of inverters can run Helios
without touching code. HAOS stays the source of truth for live data: Helios
consumes, it does not talk to the inverters directly.

## Phase 4 — Multi-installation

One Helios instance watching a single installation is fine for one home. Phase 4
explores monitoring several installations from one place: either one Helios
talking to several HAOS instances, or several Helios instances exposing their
data over an API and aggregating into a single view.
