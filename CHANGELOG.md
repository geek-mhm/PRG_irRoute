# Changelog

## 0.1.3 - 2026-09-12

- Added source-install support for AlmaLinux and other dnf-based systems.
- Added persistent loose reverse-path filtering for dual-interface routing.
- Added the reverse-path filtering configuration to offline release bundles.
- Expanded installation, setup, validation, update, and WHM documentation.

## 0.1.2 - 2026-09-12

- Added managed DNS profiles using `systemd-resolved`.
- Added setup wizard and CLI controls for international DNS servers.
- Added DNS restoration on disable and rollback.

## 0.1.1 - 2026-09-12

- Fixed managed rule priorities so irroute policy runs before Linux's standard main-table rule.
- Added a main-table non-default lookup to preserve Docker, LAN, VPN, and static routes.
- Added automatic migration from the unsafe 0.1.0 priority defaults.

## 0.1.0 - 2026-09-12

- Added the phase-one policy-routing engine.
- Added A/B route-table activation and a dedicated public-source route.
- Added the English interactive menu and setup wizard.
- Added CIDR data import and force-route management.
- Added diagnostics, status, snapshots, rollback, and connectivity confirmation.
- Added systemd persistence, source installation, and unit tests.
- Added dependency installation and a clean Ubuntu server test procedure.
- Added bundled Iran IPv4 seed data with documented provenance.
- Added dependency-free offline bundles for Linux amd64 and arm64 servers.
