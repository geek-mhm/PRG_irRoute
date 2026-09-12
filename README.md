# irroute

`irroute` is a policy-routing manager for Linux hosts with two IPv4 internet paths. Iran destination networks use the local public interface, while all other host-generated IPv4 traffic uses the international interface.

The project replaces ad-hoc routing scripts with validated configuration, a bundled Iran IPv4 list, A/B routing tables, force-route overrides, managed DNS, diagnostics, snapshots, automatic connectivity rollback, and systemd boot persistence. The interactive interface, source code, comments, and documentation are English-only.

## Routing model

The setup wizard uses these terms:

- **Local interface:** the public interface and gateway used for Iran destinations.
- **International interface:** the interface and gateway used for all other destinations.
- **Force-local:** an address or prefix that must always use the local interface.
- **Force-international:** an address or prefix that must always use the international interface, even when it is present in the Iran list.

The application manages host-generated IPv4 traffic. It does not currently manage forwarded client traffic, IPv6, NAT, firewall policy, or gateway failover.

## Safety model

- The application owns only its configured routing-rule priorities and route tables.
- Existing main-table routes for connected networks, Docker bridges, VPNs, and static routes are preserved.
- Traffic sourced from the public local address always returns through the local public gateway.
- A new route generation is loaded into an inactive A/B table before the active rule is switched.
- Every apply, disable, data import, and configuration change creates a snapshot.
- Interactive activation schedules an independent systemd rollback timer before changing routes.
- The operator must type `CONFIRM` within 90 seconds or the previous snapshot is restored after 120 seconds.
- Existing resources at irroute priorities or table IDs are treated as conflicts during first activation.
- Interface MAC addresses are pinned to detect accidental interface replacement.
- Installers enable loose reverse-path filtering, which is required for reliable dual-interface policy routing.

Keep provider console access and two SSH sessions open during the first activation on every server.

## Supported systems

- Ubuntu 22.04 and 24.04
- AlmaLinux 8 and 9, including WHM/cPanel hosts
- Other systemd-based distributions with compatible `iproute2` may work but are not currently tested
- Linux `amd64` and `arm64` release bundles

Required runtime commands are `ip`, `systemctl`, and `systemd-run`. Managed DNS additionally requires an active `systemd-resolved` service and the `resolvectl` command.

Source installation requires Git and Go 1.18 or newer. The source installer can install missing build packages through `apt-get` or `dnf`. Offline release bundles do not require Git, Go, or package downloads.

## Inspect the server before installation

Record the current network state:

```sh
ip -br -4 address
ip -4 route show
ip -4 rule show
printf 'SSH client: %s\n' "$SSH_CLIENT"
```

Confirm that both gateways are directly reachable. Replace the example interfaces and gateways:

```sh
ping -I LOCAL_INTERFACE -c 2 LOCAL_GATEWAY
ping -I INTERNATIONAL_INTERFACE -c 2 INTERNATIONAL_GATEWAY
```

Do not continue until the interfaces, addresses, and gateways are known.

## Installation method 1: clone and build from GitHub

Use this method on a server that can reach GitHub and its operating-system package repositories:

```sh
git clone https://github.com/geek-mhm/PRG_irRoute.git
cd PRG_irRoute
sudo ./scripts/install.sh
```

The installer:

- detects `apt-get` or `dnf`;
- installs missing Go, iproute2, and systemd packages;
- runs the test suite;
- builds and installs `/usr/local/sbin/irroute`;
- installs the systemd unit;
- installs the loose reverse-path-filtering configuration;
- imports the bundled Iran IPv4 seed only when no existing data is present;
- preserves existing irroute configuration and data during an upgrade;
- does not activate routing or enable boot persistence.

For a reproducible source build, check out a release tag before installing:

```sh
git fetch --tags
git checkout v0.1.4
sudo ./scripts/install.sh
```

## Installation method 2: download a release from GitHub

This method avoids installing a compiler. Use `amd64` for `x86_64` servers and `arm64` for `aarch64` servers.

```sh
mkdir -p /root/irroute-release-v0.1.4
cd /root/irroute-release-v0.1.4

curl -fLO https://github.com/geek-mhm/PRG_irRoute/releases/download/v0.1.4/irroute-0.1.4-linux-amd64.tar.gz
curl -fLO https://github.com/geek-mhm/PRG_irRoute/releases/download/v0.1.4/SHA256SUMS

sha256sum -c SHA256SUMS --ignore-missing
tar -xzf irroute-0.1.4-linux-amd64.tar.gz
cd irroute-0.1.4-linux-amd64
sudo ./install.sh
```

Never install a release archive when checksum validation fails.

## Installation method 3: isolated server without international internet

Download the release bundle and `SHA256SUMS` on a connected computer. Transfer both files through the existing management path:

```sh
scp -P SSH_PORT irroute-0.1.4-linux-amd64.tar.gz root@SERVER_IP:/root/
scp -P SSH_PORT SHA256SUMS root@SERVER_IP:/root/
```

On the target server:

```sh
cd /root
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf irroute-0.1.4-linux-amd64.tar.gz
cd irroute-0.1.4-linux-amd64
./install.sh
```

The release archive contains the static binary, systemd unit, reverse-path-filtering configuration, Iran IPv4 seed, license, and offline installer.

## First setup

Run the wizard without changing live routes:

```sh
sudo irroute setup
```

The wizard asks for:

1. local public interface;
2. local interface IPv4 address in CIDR notation;
3. local gateway;
4. international interface;
5. international interface IPv4 address in CIDR notation;
6. international gateway;
7. DNS mode;
8. Iran CIDR source file.

Press Enter to accept a detected value. The bundled Iran data path is offered automatically on a new installation.

### DNS mode

Choose `managed` when `systemd-resolved` is active. Managed mode assigns the configured DNS servers and the route-only `~.` domain to the international interface. Disable and rollback restore the network-provided per-link DNS configuration.

Choose `system` on WHM/cPanel installations where DNS is managed through `/etc/resolv.conf`, NetworkManager, or another service and `systemd-resolved` is inactive.

Check resolver availability with:

```sh
systemctl is-active systemd-resolved.service
command -v resolvectl
```

## WHM/cPanel example

The following topology is an example only. Use the values reported by the server:

```text
Local interface: ens192
Local address: 77.238.108.91/29
Local gateway: 77.238.108.89

International interface: ens224
International address: 192.168.10.11/24
International gateway: 192.168.10.1

DNS mode: system
Iran CIDR source: accept the default
```

Using `system` DNS mode prevents irroute from changing resolver ownership on a WHM server. Policy routing still sends normal non-Iran DNS traffic through the international interface.

## Validate before activation

```sh
sudo irroute version
sudo irroute doctor
sudo irroute status
sudo irroute plan
```

For a detailed route audit:

```sh
sudo irroute plan --commands
```

The plan must show:

- a main non-default lookup before the policy table;
- a public-source rule pointing to the local source table;
- an inactive A/B target table;
- Iran prefixes through the local interface;
- the default route through the international interface.

## Activate safely

Open two SSH sessions and keep provider console access available. Start activation in the first session:

```sh
sudo irroute enable
```

Use the second session to test the live policy. Replace the Iran test address when required:

```sh
ip -4 rule show
ip -4 route get 2.176.241.45
ip -4 route get 8.8.8.8
getent ahostsv4 api.ipify.org
curl -4 --max-time 15 -fsS https://api.ipify.org
```

For a public server, also verify SSH, HTTP, HTTPS, and its management panel from another machine. Return to the first session and enter:

```text
CONFIRM
```

If any test fails, do not confirm. The scheduled systemd timer restores the previous snapshot automatically.

`--no-confirm` disables this protection and should be used only by an external deployment system with its own health checks and rollback:

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
sudo irroute plan
```

Disable managed routing and boot persistence:

```sh
sudo irroute disable
```

Restore the latest safety snapshot:

```sh
sudo irroute rollback latest
```

## Force-route management

Force one address or prefix through the local public path:

```sh
sudo irroute force local add 203.0.113.20 --label "Monitoring endpoint"
sudo irroute force local list
sudo irroute force local remove 203.0.113.20
```

Force an Iran destination through the international path:

```sh
sudo irroute force international add 198.51.100.0/24 --label "International override"
sudo irroute force international list
sudo irroute force international remove 198.51.100.0/24
```

When routing is enabled, force-route changes create a new A/B generation and require connectivity confirmation.

## DNS management

Show the configured DNS policy:

```sh
sudo irroute dns show
```

Enable managed DNS through the international interface:

```sh
sudo irroute dns set 1.1.1.1 8.8.8.8
```

Return DNS ownership to the system network configuration:

```sh
sudo irroute dns system
```

Changing DNS while routing is enabled requires connectivity confirmation.

## Iran CIDR data

The repository and every release bundle contain a validated Iran IPv4 seed. A replacement file may contain one IPv4 address or CIDR per line. Blank lines and comments beginning with `#` are accepted.

Import a replacement list:

```sh
sudo irroute data import /path/to/iran-ipv4.cidr
```

The application normalizes, deduplicates, validates, and hashes the list. When routing is enabled, importing data creates a new A/B generation and requires confirmation.

## Updating irroute

There is not yet a built-in `irroute update` command. Installation over an existing version preserves `/etc/irroute/config.json`, active Iran data, state, and snapshots.

Update a source installation:

```sh
cd /path/to/PRG_irRoute
git fetch --tags
git checkout v0.1.4
sudo ./scripts/install.sh
sudo irroute doctor
```

Update a release installation by downloading the new archive and checksum, validating them, and running its `install.sh`. If routing is already enabled, apply the new version with confirmation:

```sh
sudo irroute apply
```

## Files and ownership

| Path | Purpose |
| --- | --- |
| `/etc/irroute/config.json` | Versioned configuration |
| `/etc/sysctl.d/90-irroute.conf` | Loose reverse-path-filtering prerequisite |
| `/var/lib/irroute/state.json` | Active generation state |
| `/var/lib/irroute/data/iran-current.cidr` | Validated Iran CIDR data |
| `/var/lib/irroute/backups/` | Rollback snapshots |
| `/run/irroute/` | Runtime lock data |
| `/usr/local/sbin/irroute` | Application binary |
| `/etc/systemd/system/irroute.service` | Boot persistence unit |

Configuration and state directories use mode `0700`; application data files use mode `0600`.

## Troubleshooting

If `doctor` reports a rule or table collision, inspect the resources before removing anything:

```sh
ip -4 rule show
ip -4 route show table 51810
ip -4 route show table 51820
ip -4 route show table 51821
```

If managed DNS fails, either start and correctly configure `systemd-resolved` or switch to system DNS:

```sh
sudo irroute dns system
```

If boot activation fails, use provider console access and disable persistence:

```sh
systemctl disable --now irroute.service
```

Then inspect:

```sh
journalctl -u irroute.service --no-pager -n 100
sudo irroute doctor
```

Avoid deleting routing rules, route tables, firewall objects, or network configuration unless their ownership is known.

## Development

```sh
make test
make build
./bin/irroute help
```

Use `IRROUTE_ROOT` to redirect application files into a temporary root during development. Live route application still requires Linux and root privileges.

```sh
IRROUTE_ROOT=/tmp/irroute-test ./bin/irroute setup
```

Additional documentation:

- [Architecture](docs/architecture.md)
- [Offline installation](docs/offline-install.md)
- [Clean-server test procedure](docs/test-server.md)
- [Roadmap](docs/roadmap.md)
