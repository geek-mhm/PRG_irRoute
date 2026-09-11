# irroute

`irroute` is an Ubuntu-first policy-routing manager for hosts with separate local and international internet paths. It sends Iran destination networks through the local public interface and sends all other host-generated traffic through the international interface.

Phase one provides a safe, testable routing core and an English-only SSH interface. It replaces ad-hoc boot scripts with versioned configuration, validated CIDR data, isolated routing tables, A/B route activation, snapshots, diagnostics, and rollback.

## Safety model

- The application owns only its configured rule priorities and route tables.
- A new route generation is loaded into the inactive A/B table before the active rule is switched.
- The public source address always has a higher-priority rule that returns traffic through the local public gateway. This protects services reached through the server's public IP.
- Non-default routes from the system main table are preserved before irroute policy is evaluated, protecting Docker, LAN, VPN, and static routes.
- Every apply, disable, data import, and configuration change creates a rollback snapshot.
- Interactive activation schedules an independent systemd rollback. The operator must type `CONFIRM` within 90 seconds or the previous snapshot is restored after 120 seconds.
- Existing rules at irroute priorities are treated as conflicts during first activation and are never silently overwritten.
- Interface MAC addresses can be pinned by the setup wizard to detect unexpected interface identity changes.

## Requirements

- Ubuntu 22.04 or 24.04
- Two already-configured IPv4 interfaces
- `iproute2`
- Root privileges for route changes
- Go 1.18 or newer when installing from source

Phase one manages host-generated IPv4 traffic only. Forwarded client traffic, IPv6, automated gateway health failover, application updates, and split DNS policy are planned for later phases.

The repository includes a validated Iran IPv4 seed under `data/`. Offline release bundles include this seed automatically, so a separate CIDR download is not required for the first installation.

## Install from source on Ubuntu

```sh
sudo apt-get update
sudo apt-get install -y git
git clone https://github.com/geek-mhm/PRG_irRoute.git
cd PRG_irRoute
sudo ./scripts/install.sh
```

The installer installs missing build dependencies through `apt-get`, runs the test suite, builds `/usr/local/sbin/irroute`, installs the systemd unit, and creates protected configuration and state directories. It does not change routes or enable the service.

## First setup

The installer imports the bundled Iran IPv4 seed automatically on a new server. The setup wizard offers that file as its default, so press Enter to keep it. You may instead provide a custom file containing one IPv4 CIDR per line.

```sh
sudo irroute setup
sudo irroute doctor
sudo irroute plan
sudo irroute enable
```

The setup wizard asks which interface is local, which interface is international, their addresses and gateways, and the Iran CIDR source path. Review the plan before activation.

After `enable` changes the active route table, verify the current SSH session and both egress paths, then type `CONFIRM`. For unattended deployment only, `--no-confirm` disables this protection:

```sh
sudo irroute enable --no-confirm
```

## Common operations

Open the interactive menu:

```sh
sudo irroute
```

Inspect status and diagnostics:

```sh
sudo irroute status
sudo irroute doctor
sudo irroute plan --commands
```

Force one address or network through the public local path:

```sh
sudo irroute force local add 203.0.113.20 --label "Monitoring endpoint"
sudo irroute force local list
sudo irroute force local remove 203.0.113.20
```

Force an Iran destination through the international path:

```sh
sudo irroute force international add 198.51.100.0/24 --label "International override"
```

Import a replacement Iran CIDR file:

```sh
sudo irroute data import /path/to/iran-ipv4.cidr
```

If routing is enabled, force-route and data changes create a new A/B generation and require connectivity confirmation.

Disable only the routes and rules managed by irroute:

```sh
sudo irroute disable
```

Restore the latest safety snapshot:

```sh
sudo irroute rollback latest
```

## Files and ownership

| Path | Purpose |
| --- | --- |
| `/etc/irroute/config.json` | Versioned configuration |
| `/var/lib/irroute/state.json` | Active generation state |
| `/var/lib/irroute/data/iran-current.cidr` | Validated Iran CIDR data |
| `/var/lib/irroute/backups/` | Rollback snapshots |
| `/usr/local/sbin/irroute` | Application binary |
| `/etc/systemd/system/irroute.service` | Boot persistence unit |

Configuration and state directories use mode `0700`; files use mode `0600`.

## Development

```sh
make test
make build
./bin/irroute help
```

Use `IRROUTE_ROOT` to redirect application files into a temporary root during development. Route application still requires Linux and root privileges.

```sh
IRROUTE_ROOT=/tmp/irroute-test ./bin/irroute setup
```

See [docs/architecture.md](docs/architecture.md) for routing behavior and [docs/roadmap.md](docs/roadmap.md) for the planned update, DNS, and health-management phases.

For a complete clean-server test procedure, see [docs/test-server.md](docs/test-server.md).

For servers without international internet access, see [docs/offline-install.md](docs/offline-install.md). The offline bundle requires no Git client, Go compiler, or package download on the target server.
