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

if command -v apt-get >/dev/null 2>&1; then
    package_manager=apt
elif command -v dnf >/dev/null 2>&1; then
    package_manager=dnf
else
    echo "Error: source installation requires apt-get or dnf." >&2
    exit 1
fi

set --
if ! command -v go >/dev/null 2>&1; then
    if [ "$package_manager" = apt ]; then
        set -- "$@" golang-go
    else
        set -- "$@" golang
    fi
fi
if ! command -v ip >/dev/null 2>&1; then
    if [ "$package_manager" = apt ]; then
        set -- "$@" iproute2
    else
        set -- "$@" iproute
    fi
fi
if ! command -v systemctl >/dev/null 2>&1; then
    set -- "$@" systemd
fi
if ! command -v sysctl >/dev/null 2>&1; then
    if [ "$package_manager" = apt ]; then
        set -- "$@" procps
    else
        set -- "$@" procps-ng
    fi
fi

if [ "$#" -gt 0 ]; then
    echo "Installing required packages: $*"
    if [ "$package_manager" = apt ]; then
        apt-get update
        apt-get install -y "$@"
    else
        dnf install -y "$@"
    fi
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
build_dir=$(mktemp -d "$project_dir/.irroute-build.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
mkdir -p "$build_dir/go-cache" "$build_dir/go-tmp"
GOCACHE="$build_dir/go-cache"
GOTMPDIR="$build_dir/go-tmp"
export GOCACHE GOTMPDIR

cd "$project_dir"
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=0.1.4" -o "$build_dir/irroute" ./cmd/irroute

install -D -m 0755 "$build_dir/irroute" /usr/local/sbin/irroute
install -D -m 0644 packaging/irroute.service /etc/systemd/system/irroute.service
install -D -m 0644 packaging/90-irroute.conf /etc/sysctl.d/90-irroute.conf
install -d -m 0700 /etc/irroute /var/lib/irroute/data /var/lib/irroute/backups /run/irroute

if command -v sysctl >/dev/null 2>&1; then
    sysctl -p /etc/sysctl.d/90-irroute.conf >/dev/null
fi

if [ ! -f /var/lib/irroute/data/iran-current.cidr ]; then
    /usr/local/sbin/irroute data import "$project_dir/data/iran-ipv4.cidr"
    echo "Bundled Iran IPv4 seed data was imported."
else
    echo "Existing Iran IPv4 data was preserved."
fi

systemctl daemon-reload

echo "irroute 0.1.4 was installed successfully."
echo "No routing changes were made. Run: sudo irroute setup"
