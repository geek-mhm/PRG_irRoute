VERSION ?= 0.1.0-dev
BINARY := bin/irroute

.PHONY: build test vet check clean install offline-bundles

build:
	mkdir -p bin
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/irroute

test:
	go test ./...

vet:
	go vet ./...

check: test vet build

clean:
	rm -f $(BINARY)

install: build
	install -D -m 0755 $(BINARY) /usr/local/sbin/irroute
	install -D -m 0644 packaging/irroute.service /etc/systemd/system/irroute.service
	install -d -m 0700 /etc/irroute /var/lib/irroute/data /var/lib/irroute/backups /run/irroute
	systemctl daemon-reload

offline-bundles:
	VERSION=$(VERSION) ./scripts/build-offline-bundles.sh
