# Clean Linux Server Test Procedure

This procedure is intended for a disposable Ubuntu 22.04/24.04 or AlmaLinux 8/9 server with two already-configured IPv4 interfaces. Keep provider console access available during the first routing test.

If the server cannot reach GitHub or its package repositories, follow [offline-install.md](offline-install.md) instead of the source installation steps below.

## 1. Inspect the server before installation

Record the interface names, addresses, gateways, current default route, and SSH source address:

```sh
ip -br -4 address
ip -4 route show
ip -4 rule show
printf 'SSH client: %s\n' "$SSH_CLIENT"
```

Do not continue until both interfaces have their intended static addresses and each gateway is known.

## 2. Clone and install

```sh
sudo apt-get update
sudo apt-get install -y git
git clone https://github.com/geek-mhm/PRG_irRoute.git
cd PRG_irRoute
sudo ./scripts/install.sh
```

Installation does not activate policy routing.

Verify the binary and service:

```sh
sudo irroute version
systemctl cat irroute.service
```

## 3. Review or replace Iran IPv4 data

The installer imports the bundled Iran IPv4 seed on a new server. No separate download is required. To use a custom list instead, copy it to the test server:

```sh
scp /path/to/iran-ipv4.cidr root@SERVER_IP:/root/iran-ipv4.cidr
```

Custom files must contain one IPv4 address or CIDR per line. Blank lines and comments starting with `#` are accepted.

## 4. Run setup without changing routes

```sh
sudo irroute setup
```

Select the public Iran interface as `local`. Select the unfiltered path as `international`. Enter each interface address in CIDR notation and its directly reachable gateway. Select managed DNS when `systemd-resolved` is active. Select system DNS on WHM/cPanel servers where another service owns resolver configuration. Press Enter at the data-source prompt to use the bundled seed, or enter `/root/iran-ipv4.cidr` if you copied a custom file.

## 5. Validate the generated policy

```sh
sudo irroute doctor
sudo irroute status
sudo irroute plan
```

For a detailed audit, save the generated commands and inspect them before activation:

```sh
sudo irroute plan --commands > /root/irroute-plan.txt
less /root/irroute-plan.txt
```

The plan must show:

- a public-source rule pointing to the local source table;
- an inactive A/B target table;
- Iran prefixes through the local interface;
- a default route through the international interface.

## 6. Activate with automatic rollback protection

Keep the current SSH session open and start a second SSH session before activation. Then run:

```sh
sudo irroute enable
```

The rollback timer is scheduled before any route changes. After activation:

1. confirm both SSH sessions are responsive;
2. verify local and international egress from the second session;
3. verify DNS resolution with `getent ahostsv4 api.ipify.org`;
4. return to the first session and type `CONFIRM` exactly as shown.

If connectivity is lost or confirmation is not entered, systemd restores the pre-change snapshot after 120 seconds.

## 7. Verify the live result

```sh
sudo irroute status
sudo irroute doctor
ip -4 rule show
ip -4 route show table 51810
ip -4 route show table 51820
ip -4 route show table 51821
```

Test representative destinations while binding requests to the expected source interfaces when possible. Record the observed public egress addresses.

## 8. Disable and confirm cleanup

```sh
sudo irroute disable
sudo irroute status
ip -4 rule show
```

Disable removes only the three managed rules and flushes only the three reserved routing tables. It also disables boot persistence.

## Recovery commands

Restore the latest snapshot:

```sh
sudo irroute rollback latest
```

Disable the boot unit from provider console if boot activation fails:

```sh
systemctl disable irroute.service
```

Avoid `--no-confirm` during the first server test.
