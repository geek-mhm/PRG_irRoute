# Clean Ubuntu Test Procedure

This procedure is intended for a disposable Ubuntu 22.04 or 24.04 server with two already-configured IPv4 interfaces. Keep provider console access available during the first routing test.

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

## 3. Provide Iran IPv4 data

Copy the validated Iran CIDR file to the test server. For example, run this from the computer that holds the file:

```sh
scp /path/to/iran-ipv4.cidr root@SERVER_IP:/root/iran-ipv4.cidr
```

The file must contain one IPv4 address or CIDR per line. Blank lines and comments starting with `#` are accepted.

## 4. Run setup without changing routes

```sh
sudo irroute setup
```

Select the public Iran interface as `local`. Select the unfiltered path as `international`. Enter each interface address in CIDR notation and its directly reachable gateway. Use `/root/iran-ipv4.cidr` as the data source.

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
3. return to the first session and type `CONFIRM` exactly as shown.

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

Disable removes only the two managed rules and flushes only the three reserved routing tables. It also disables boot persistence.

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
