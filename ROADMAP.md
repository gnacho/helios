# Roadmap

This is a personal project. The roadmap reflects the direction I have in mind,
not promises or dates. Phases 1 and 2 are done; 3 is mostly done; 4 is an idea
I want to explore.

| Phase | Focus | Status |
|---|---|---|
| 1 | Live read-only panel (production vs consumption) | Done |
| 2 | Real backend, multi-user, history, battery, alerts, PWA, hardening | Done (~0.6.x) |
| 3 | Configurable hardware (still under HAOS) | Mostly done (0.7.x) |
| 4 | Multi-installation: several HAOS or several Helios via API | Exploring |

## Phase 3 — Configurable hardware, still under HAOS

The installation topology (inverters, panels, battery, grid source) and the
HAOS sensor mapping used to be hardcoded for one setup. Since 0.7.x it is
resolved from `install_config` (a JSON in the kv store) and can be edited
without touching code:

- N inverters (each with its own power/energy sensors and units), optional
  battery with language-neutral charge/discharge states, and a configurable
  grid source: the Solis Cloud scraper **or** plain grid sensors.
- `GET /api/install` returns the resolved topology + entities; the UI reads it
  instead of hardcoding entity IDs.
- Fresh installs get a generic profile (no scraper); existing installs keep
  the legacy Solis/Fox/scraper profile automatically, so nothing breaks.

HAOS stays the source of truth for live data: Helios consumes, it does not
talk to the inverters directly. Remaining gap: no UI yet to *edit* the topology
in Settings (config is written via the API/config endpoint or env).

## Phase 4 — Multi-installation

One Helios instance watching a single installation is fine for one home. Phase 4
explores monitoring several installations from one place: either one Helios
talking to several HAOS instances, or several Helios instances exposing their
data over an API and aggregating into a single view.
