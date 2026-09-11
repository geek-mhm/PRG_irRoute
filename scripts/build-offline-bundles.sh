#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
version=${VERSION:-0.1.0}
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM

cd "$project_dir"

if [ ! -f data/iran-ipv4.cidr ]; then
    echo "Error: bundled Iran IPv4 data is missing." >&2
    exit 1
fi

go test ./...
mkdir -p dist

IRROUTE_ROOT="$build_dir/validation-root" go run ./cmd/irroute data import data/iran-ipv4.cidr >/dev/null

for architecture in amd64 arm64; do
    package_name="irroute-${version}-linux-${architecture}"
    package_dir="$build_dir/$package_name"
    mkdir -p "$package_dir"

    CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go build \
        -trimpath \
        -ldflags "-s -w -X main.version=$version" \
        -o "$package_dir/irroute" \
        ./cmd/irroute

    cp data/iran-ipv4.cidr "$package_dir/iran-ipv4.cidr"
    cp data/README.md "$package_dir/DATA-SOURCE.md"
    cp packaging/irroute.service "$package_dir/irroute.service"
    cp packaging/install-offline.sh "$package_dir/install.sh"
    cp LICENSE "$package_dir/LICENSE"
    chmod 0755 "$package_dir/install.sh" "$package_dir/irroute"

    tar -C "$build_dir" -czf "dist/$package_name.tar.gz" "$package_name"
done

if command -v sha256sum >/dev/null 2>&1; then
    (cd dist && sha256sum irroute-"$version"-linux-*.tar.gz > SHA256SUMS)
else
    (cd dist && shasum -a 256 irroute-"$version"-linux-*.tar.gz > SHA256SUMS)
fi

echo "Offline bundles were created in $project_dir/dist"
