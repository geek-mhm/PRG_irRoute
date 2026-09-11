# Offline Installation

Use an offline bundle when the target server cannot reach GitHub or Ubuntu package mirrors. The bundle contains a static Linux binary, the systemd unit, the Iran IPv4 seed, and an installer that does not use the network.

## Obtain a bundle on a connected computer

Download a prebuilt bundle and its checksum from the matching GitHub release, or build it from the project repository.

Example release download for an `x86_64` server:

```sh
curl -LO https://github.com/geek-mhm/PRG_irRoute/releases/download/v0.1.1/irroute-0.1.1-linux-amd64.tar.gz
curl -LO https://github.com/geek-mhm/PRG_irRoute/releases/download/v0.1.1/SHA256SUMS
```

To build locally instead:

From the project repository:

```sh
make offline-bundles VERSION=0.1.1
```

This creates:

- `dist/irroute-0.1.1-linux-amd64.tar.gz` for `x86_64` servers;
- `dist/irroute-0.1.1-linux-arm64.tar.gz` for `aarch64` servers;
- `dist/SHA256SUMS` for transfer verification.

No Go compiler or Git client is needed on the target server in either case.

## Identify the server architecture

On the target server:

```sh
uname -m
```

Use the `amd64` bundle for `x86_64`. Use the `arm64` bundle for `aarch64` or `arm64`.

## Transfer the bundle

Run from the connected computer and replace the example values:

```sh
scp dist/irroute-0.1.1-linux-amd64.tar.gz root@SERVER_IP:/root/
scp dist/SHA256SUMS root@SERVER_IP:/root/
```

SCP transfers over the existing SSH connection and does not require the target server to have internet access.

## Verify and install on the target server

```sh
cd /root
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf irroute-0.1.1-linux-amd64.tar.gz
cd irroute-0.1.1-linux-amd64
./install.sh
```

The offline installer:

- installs the prebuilt binary at `/usr/local/sbin/irroute`;
- installs the systemd service;
- imports the bundled Iran IPv4 seed only when no existing irroute data is present;
- does not enable or apply policy routing.

Continue with:

```sh
irroute setup
irroute doctor
irroute plan
```

During setup, the bundled data path is already available as the default. Press Enter to keep it.

Keep two SSH sessions and provider console access open for the first `irroute enable`. Do not use `--no-confirm` during the initial test.

## Bootstrap limitation

An isolated server must receive the first bundle through an existing management channel such as SCP, SFTP, provider console upload, an internal package mirror, or an Iran-reachable web endpoint. No application can download its own first installer without at least one reachable delivery path.

After irroute successfully activates the international path, later online updates may use GitHub. A dedicated signed update mechanism is planned for phase two.
