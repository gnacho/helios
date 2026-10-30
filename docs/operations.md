# Operations

This is the short version of how to run Helios without breaking it. The only
irreplaceable part of the system is the SQLite file: everything else can be
rebuilt from the repository.

## What a deployment looks like

```
/opt/helios/
├── server/helios-go        binary
├── server/.env             environment, survives updates
├── data/helios.db          the database
├── data/                   scratch space for exports and update requests
└── .release-id             installed release id
```

The service runs as a dedicated non-root user, usually `helios`, with the
database outside the code directory so a deploy never touches it.

```ini
[Unit]
Description=Helios solar monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=helios
Group=helios
WorkingDirectory=/opt/helios/server
EnvironmentFile=/opt/helios/server/.env
ExecStart=/opt/helios/server/helios-go
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/helios/data

[Install]
WantedBy=multi-user.target
```

Save it as `/etc/systemd/system/helios.service`, or keep whatever name your
installation already uses. The author's own container calls it
`helios-go.service`.

## Build

The Makefile lives at the repository root, not inside `server-go/`:

```bash
make build
```

That regenerates the templ components, rebuilds the Tailwind CSS and compiles
the binary. It needs the `templ` CLI in `PATH` or at `~/go/bin/templ` (the
Makefile looks there). There is no Node step.

For a cross-compiled release binary:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -C server-go -trimpath -ldflags "-s -w" -o /tmp/helios-go ./cmd/server
```

## Deploy a new binary

Do these steps in this order.

```bash
scp /tmp/helios-go root@host:/opt/helios/server/helios-go.new
ssh root@host '
  cp -p /opt/helios/server/helios-go /opt/helios/server/helios-go.bak-$(date +%Y%m%d-%H%M)
  chmod 755 /opt/helios/server/helios-go.new
  mv -f /opt/helios/server/helios-go.new /opt/helios/server/helios-go
  echo 1.2.6 > /opt/helios/.release-id
  systemctl restart helios.service
'
```

Two details matter here:

- **`chmod 755` before the `mv`.** A file copied over SCP arrives as `644`, the
  service user cannot execute it, and systemd fails with `203/EXEC` in a loop.
- **Update `.release-id`.** `/api/version` reports it as `build`. If you forget
  it, the app will claim to run the previous release even though the binary is
  new.

Do not skip the `mv`. Copying a backup, changing permissions and
restarting without the `mv` leaves the old binary in place and looks like a
successful deploy.

## Automatic updates

The app can update itself from the GitHub releases. The mechanism is a systemd
path unit that watches for a request file, not a timer:

```bash
systemctl enable --now helios-update.path
```

When an admin asks for an update from the UI, the app writes
`/opt/helios/data/.update-requested` and the path unit runs the update script.
The release marker is what tells the UI which version is installed.

To roll back, stop the service, restore the previous binary from the backup
alongside it, set `.release-id` back to that version and start the service. The
database is never downgraded, so a rollback across a schema change is not safe:
check the release notes before doing it.

## Backups

The database is the only data that cannot be rebuilt.

- **Litestream** replicates it continuously (every few seconds) to an SFTP
  target, with point-in-time recovery. Point `litestream.yml` at the same file
  and keep the replica on a different machine.
- **A manual copy** before any migration or manual schema work:

```bash
ssh root@host 'python3 - <<EOF
import sqlite3
src = sqlite3.connect("/opt/helios/data/helios.db")
dst = sqlite3.connect("/opt/helios/backups/helios.db.bak-$(date +%Y%m%d-%H%M)")
src.backup(dst)
dst.close(); src.close()
EOF'
```

Use the SQLite backup API as above rather than `cp` on a live database: with WAL
enabled a plain copy can miss the last transactions. Restoring is the reverse:
stop the service, move the backup into place, start it.

## Lost admin password

```bash
curl -X POST http://127.0.0.1:PORT/api/auth/recover
```

It has to run **on the host itself**; requests through a proxy are rejected on
purpose. It returns a temporary password and resets the admin one to that value,
so change it in Settings afterwards.

## Diagnosing

- `systemctl status helios.service` and `journalctl -u helios.service -n 100`.
- `GET /api/version` returns the release id and the version compiled into the
  binary.
- The Home Assistant connection logs one line at startup (`[ha] conectado a
  ...`). If it does not appear, the token or the URL is wrong.
- Live charts are server-rendered fragments, one endpoint per chart:
  `/ui/day-chart`, `/ui/chg-curve`, `/ui/bat-charts`, `/ui/inv-charts`. If a
  chart stops moving but the page works, request the endpoint directly with a
  session cookie.
- A `superfluous response.WriteHeader` message in the log was a bug in the
  streaming wrapper, fixed in 1.2.5. Seeing it means the binary is older than
  that.

## What not to do

- Do not point the service at the database file with a relative path. `DATA_DIR`
  or `HELIOS_DATA_DIR` must be absolute.
- Do not run it as root. It does not need privileges and the unit is written
  for a normal user.
- Do not edit the database by hand while the service is running.
- Do not deploy a binary built for another architecture. The release assets are
  amd64 and arm64.
