# Configuration

Helios reads everything from Home Assistant. It never talks to an inverter,
a battery or a charger directly, so the only requirement is that HAOS already
exposes the sensors you care about.

Two layers are configured separately:

1. **The process**, through environment variables. Connection to HAOS,
   database location, prices, push notifications.
2. **The installation**, through the topology. Which entity is which inverter,
   which one is the battery, where the grid comes from.

An admin can edit the topology from Settings, with no code and no JSON. The API
accepts it too, which is what this document describes.

## Environment variables

| Variable | Default | What it does |
|---|---|---|
| `HAOS_URL` | a development address | Base URL of Home Assistant. Use `https://` outside loopback: cleartext `http://` and `ws://` are rejected unless the host is loopback. The scheme is upgraded to `wss://` or `ws://` internally. |
| `HAOS_TOKEN` | empty | Long-lived access token, generated in HAOS under Profile → Security → Long-lived access tokens. Required. |
| `HAOS_ALLOW_INSECURE` | `0` | Set to `1` only for a lab setup, to accept cleartext `http://` or `ws://` towards a non-loopback host. |
| `HELIOS_DATA_DIR` | `~/.local/share/helios` | Where the database lives. `DATA_DIR` is still accepted for compatibility with the old Node deployment. |
| `HELIOS_DB_PATH` | `<data dir>/helios.db` | Full path to the SQLite file, when you want it somewhere else. |
| `PORT` | `8080` | HTTP port. |
| `SESSION_SECRET` | empty | Secret used to sign session cookies. If it changes, every open session is invalidated. |
| `TRUSTED_PROXIES` | empty | Comma-separated IPs or CIDR ranges of reverse proxies. Empty means `X-Forwarded-For` is ignored and the direct peer address is used. Behind a proxy, set it to the proxy address so login rate limiting sees the real client IP. The proxy must also preserve the original `Host` header: mutating browser requests are rejected when `Origin` or `Referer` points to a different host. |
| `AUTH_USER` / `AUTH_PASS` | empty | Only used to create the first admin account on an empty database. Ignored once a user exists. |
| `PRICE_IMPORT_EUR` | `0.15` | Default price per kWh imported from the grid, used when no tariff is configured in the app. |
| `PRICE_EXPORT_EUR` | `0.08` | Default price per kWh exported. |
| `CO2_KG_PER_KWH` | `0.25` | Conversion factor for the CO₂ avoided figure. |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` | empty | Web push credentials. Without them the app runs, but push notifications are disabled. |
| `GITHUB_REPO` | `gnacho/helios` | Repository the in-app updater checks for new releases. |
| `RELEASE_MARKER` | `/opt/helios/.release-id` | File holding the installed release id. `/api/version` reports it as `build`. A manual deploy must update it. |
| `HELIOS_HA_DEBUG` | empty | Verbose logging of the Home Assistant connection. |
| `HELIOS_NAV_LOG` | empty | Logs navigation events, for debugging the UI. |
| `HELIOS_NETWORK_TESTS` | empty | Enables tests that need real network access. |

There is no sample environment file in the repository. Write only the variables
you need, usually `HAOS_URL`, `HAOS_TOKEN`, `SESSION_SECRET` and `HELIOS_DATA_DIR`.

## Topology

The topology is stored as JSON under the `install_config` key in the kv store.
On first boot Helios resolves it automatically: an existing database keeps its
legacy profile, an empty one gets a generic profile with no scraper and plain
grid sensors.

To set your own sensors, send a `topology` object:

```
PUT /api/config  (admin session)
{
  "topology": {
    "inverters": [
      {
        "key": "inv1",
        "name": "Roof inverter",
        "kwp": 5.0,
        "powerId": "sensor.inverter_power",
        "powerUnit": "kW",
        "energyId": "sensor.inverter_energy_today",
        "energyAcc": "state",
        "deepIds": ["sensor.inverter_energy_total"]
      }
    ],
    "battery": {
      "enabled": true,
      "powerId": "sensor.battery_power",
      "socId": "sensor.battery_soc",
      "chargingStates": ["charging"],
      "dischargingStates": ["discharging"]
    },
    "grid": { "mode": "sensor", "sensorId": "sensor.grid_net" },
    "consumption": {
      "powerIds": ["sensor.house_power"],
      "powerUnit": "W",
      "energyIds": ["sensor.house_energy"]
    },
    "sun": "sun.sun",
    "weather": "weather.forecast"
  }
}
```

`GET /api/install` returns the resolved topology and the entity list that
Settings shows.

### Inverters

An arbitrary number of inverters, each with its own key, name, peak power and
sensors. `powerUnit` is `kW` or `W`. `energyId` is the daily energy counter and
`energyAcc` says how to read it (`state` uses the value as is, other modes
accumulate deltas). `deepIds` accepts extra entities you want kept in the
history.

### Battery

One battery per installation. `chargingStates` and `dischargingStates` accept
any string, so HAOS instances in languages other than Spanish or English work
without translation. Several batteries are not supported yet, see
[#197](https://github.com/gnacho/helios/issues/197).

### Grid

- `mode: "sensor"` reads plain grid sensors.
- `mode: "attrs"` reads power and direction from the attributes of one sensor.
  This is how a scraper that publishes a single entity fits in: it is just
  another sensor.

### Chargers

EV chargers are configured separately, as extensions, because they are a
different shape of device: a switch, a power sensor, a current selector and
sometimes a schedule. The fields per charger are:

| Field | Meaning |
|---|---|
| `key`, `name` | Identifier and label |
| `powerId`, `powerUnit` | Instant power sensor, `kW` or `W` |
| `energyTotalId`, `energyDivisor` | Lifetime energy counter |
| `energySessionId` | Energy of the current session, if the device reports it |
| `stateId` | Charger state string (charging, connected, free...) |
| `tempId` | Temperature sensor |
| `switchId` | Switch that starts and stops the charge |
| `ampId` | Current selector, a `number` entity or an `input_select` |
| `delayId`, `durationId` | Start delay and duration, for portable units |
| `scheduleStartId`, `scheduleDurationId`, `scheduleArmId`, `scheduleTimerId` | Wall box scheduling |
| `chargingStates`, `connectedStates` | The device's own vocabulary |
| `chargerInHouseMeters` | Whether the charger is already inside the house consumption meter |

Leave `energyTotalId` empty when the firmware does not really publish energy:
the card then uses the accounting Helios keeps itself, instead of showing a
constant zero. The same applies to `energySessionId`.

Charger cards show power when the reading is fresh, a "no data for N minutes"
notice when the entity has gone quiet, and "Disconnected" when every configured
entity is unavailable, which is what a portable charger looks like when it is
unplugged.

## Prices and savings

The savings figure compares each self-consumed kWh against the import price you
configure, and each exported kWh against the export price. Set them in Settings
or through `PRICE_IMPORT_EUR` and `PRICE_EXPORT_EUR` for a global default. A
flat average price gives an honest approximation; an hourly tariff is more
accurate if you have one.
