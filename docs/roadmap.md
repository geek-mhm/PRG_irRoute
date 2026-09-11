# Roadmap

## Phase one: routing core

- English interactive SSH menu and command interface
- Versioned JSON configuration
- Interface discovery and MAC pinning
- Iran CIDR import, normalization, validation, and hashing
- Force-local and force-international management
- Dedicated route ownership and A/B activation
- Enable, disable, plan, status, diagnostics, snapshots, and rollback
- systemd boot persistence and connectivity-confirmed changes
- Managed per-link DNS using `systemd-resolved`, with disable and rollback restoration
- Unit tests and source installer

## Phase two: controlled updates and DNS

- Signed release manifest and binary packages for supported Ubuntu versions
- Stable and testing update channels
- Separate application and Iran-data versions
- Download verification, staged installation, and rollback
- Scheduled data refresh with change preview

## Phase three: health and recovery

- Gateway and external-path probes bound to each interface
- Failure thresholds and hysteresis
- Manual, local-fallback, and block policies
- Structured event history and bounded logs
- Boot recovery reconciliation between saved and kernel state
- Exportable support bundle with secrets removed

## Phase four: fleet management

- Idempotent non-interactive enrollment
- Signed central policy bundles
- Fleet inventory and version reporting
- Staged rollouts and failure-domain limits
- Optional metrics and alert integrations
