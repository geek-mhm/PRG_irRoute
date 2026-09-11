# Architecture

## Routing intent

The phase-one host policy is:

1. Non-default routes already present in the system main table are preserved. This keeps connected networks, Docker bridges, VPN routes, and administrator-defined static routes working.
2. Traffic sourced from the local public address uses the dedicated local source table. This preserves symmetric replies for connections that entered through the public interface.
3. All other host-generated IPv4 traffic consults the active irroute A/B table.
4. Effective Iran destinations use the local gateway.
5. All remaining destinations use the international gateway.

`force-local` entries are added to the effective Iran destination set. `force-international` entries are subtracted from that set and receive explicit international routes. Overlapping entries in the two force lists are rejected.

## Reserved resources

The defaults are deliberately grouped in a high, application-specific range:

| Resource | Default |
| --- | ---: |
| Main non-default routes rule priority | 10000 |
| Public source rule priority | 10010 |
| Main policy rule priority | 10020 |
| Local source table | 51810 |
| A table | 51820 |
| B table | 51821 |

All managed priorities precede Linux's standard main-table rule at priority 32766. The first rule looks up the main table with `suppress_prefixlength 0`, which ignores only its default route while retaining more-specific routes.

These values are stored in configuration. First activation stops if any managed rule priority is already occupied. Disable removes only these three rules and flushes only these three tables. Configuration written by version 0.1.0 is migrated automatically to the safe priorities before activation.

## A/B activation

When table A is active, the next route generation is built in table B. When table B is active, the next generation is built in table A. The application:

1. validates configuration and CIDR data;
2. verifies interface existence, configured addresses, and pinned MAC addresses;
3. writes a safety snapshot;
4. flushes and completely loads the inactive target table;
5. updates the two local source-table routes in place without emptying the live reply path;
6. switches the managed policy rules;
7. atomically records the active generation.

A route-loading error leaves the previous active table and rule unchanged. A rule-switch or state-write error triggers a best-effort restoration of the previous managed rules.

## DNS policy

Managed DNS uses `systemd-resolved` through `resolvectl`. The configured DNS servers and the route-only `~.` domain are assigned to the international interface. That interface becomes the DNS default route, while the local interface is excluded from default DNS selection. This prevents a resolver inherited on the public local link from bypassing the international path.

The original network-provided per-link DNS configuration is not overwritten on disk. `irroute disable`, a safety rollback, or switching to system DNS calls `resolvectl revert` for both managed interfaces and flushes the resolver cache. The systemd boot service reapplies managed DNS after the network is online.

## Connectivity confirmation

For an interactive apply, the CLI creates a transient systemd timer before asking for confirmation. The timer invokes the installed irroute binary with the pre-change snapshot. This timer runs independently from the SSH session. A lost session therefore does not prevent rollback.

`apply --boot` does not schedule a confirmation timer because it runs non-interactively during boot. `--no-confirm` is available for an external deployment system that provides its own health and rollback controls.

## Data processing

Iran data accepts IPv4 addresses and CIDRs. Bare addresses become `/32` entries. Networks are masked, deduplicated, sorted, and limited to 16 MiB. The active normalized data receives a SHA-256 digest in state.

CIDR subtraction is performed in memory to make broad `force-international` entries authoritative even when the Iran list contains more-specific networks.

## Persistence

The systemd service runs `irroute apply --boot` after `network-online.target`. It is enabled only after a confirmed manual activation and disabled by `irroute disable`.

Mutating engine operations take a non-blocking process lock in `/run/irroute`. A concurrent apply, disable, or rollback is rejected instead of racing another route generation.
