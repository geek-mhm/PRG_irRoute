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

for command_name in install ip systemctl; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        echo "Error: required command '$command_name' is not available." >&2
        exit 1
    fi
done

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

install -D -m 0755 "$script_dir/irroute" /usr/local/sbin/irroute
install -D -m 0644 "$script_dir/irroute.service" /etc/systemd/system/irroute.service
install -d -m 0700 /etc/irroute /var/lib/irroute/data /var/lib/irroute/backups /run/irroute

if [ ! -f /var/lib/irroute/data/iran-current.cidr ]; then
    /usr/local/sbin/irroute data import "$script_dir/iran-ipv4.cidr"
    echo "Bundled Iran IPv4 seed data was imported."
else
    echo "Existing Iran IPv4 data was preserved."
fi

systemctl daemon-reload

installed_version=$(/usr/local/sbin/irroute version)
echo "$installed_version was installed successfully."
echo "No routing changes were made. Run: irroute setup"
