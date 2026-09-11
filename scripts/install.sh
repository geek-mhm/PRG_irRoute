#!/bin/sh
set -eu

if [ "$(uname -s)" != "Linux" ]; then
    echo "Error: irroute can only be installed on Linux." >&2
    exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: run this installer as root." >&2
    exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
    echo "Error: phase-one source installation requires Ubuntu with apt-get." >&2
    exit 1
fi

set --
if ! command -v go >/dev/null 2>&1; then
    set -- "$@" golang-go
fi
if ! command -v ip >/dev/null 2>&1; then
    set -- "$@" iproute2
fi
if ! command -v systemctl >/dev/null 2>&1; then
    set -- "$@" systemd
fi

if [ "$#" -gt 0 ]; then
    echo "Installing required packages: $*"
    apt-get update
    apt-get install -y "$@"
fi

go_version=$(go version | awk '{print $3}' | sed 's/^go//')
go_major=$(printf '%s' "$go_version" | cut -d. -f1)
go_minor=$(printf '%s' "$go_version" | cut -d. -f2)
if [ "$go_major" -lt 1 ] || { [ "$go_major" -eq 1 ] && [ "$go_minor" -lt 18 ]; }; then
    echo "Error: Go 1.18 or newer is required; found Go $go_version." >&2
    exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM

cd "$project_dir"
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o "$build_dir/irroute" ./cmd/irroute

install -D -m 0755 "$build_dir/irroute" /usr/local/sbin/irroute
install -D -m 0644 packaging/irroute.service /etc/systemd/system/irroute.service
install -d -m 0700 /etc/irroute /var/lib/irroute/data /var/lib/irroute/backups /run/irroute

systemctl daemon-reload

echo "irroute 0.1.0 was installed successfully."
echo "No routing changes were made. Run: sudo irroute setup"
