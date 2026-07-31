.PHONY: dev dev-backend dev-frontend build-frontend build-arm64 build-mipsel build-all build-local clean deploy-disk deploy-ssh

# === Разработка ===

dev:
	make -j2 dev-backend dev-frontend

dev-backend:
	go run . -config config.dev.yaml

dev-frontend:
	cd frontend && npm run dev

# === Продакшен билд ===

build-frontend:
	cd frontend && npm ci && npm run build

build-arm64: build-frontend
	GOOS=linux GOARCH=arm64 go build \
		-trimpath -ldflags="-s -w" \
		-o build/xkeen-panel-aarch64 \
		.

build-mipsel: build-frontend
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build \
		-trimpath -ldflags="-s -w" \
		-o build/xkeen-panel-mipsel \
		.

build-all: build-frontend
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o build/xkeen-panel-aarch64 .
	GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags="-s -w" -o build/xkeen-panel-mipsel .

build-local: build-frontend
	go build -o build/xkeen-panel .

# === Деплой ===

# Диск роутера = /opt/ на роутере
ROUTER_DISK ?= /Volumes/bc89cc32-b398-4623-8c4a-e166db166e0c

deploy-disk: build-arm64
	mkdir -p $(ROUTER_DISK)/etc/xkeen-panel/data
	cp build/xkeen-panel-aarch64 $(ROUTER_DISK)/sbin/xkeen-panel
	@test -f $(ROUTER_DISK)/etc/xkeen-panel/config.yaml || cp config.yaml $(ROUTER_DISK)/etc/xkeen-panel/config.yaml
	@echo "Deployed to router disk"

deploy-ssh: build-arm64
	scp build/xkeen-panel-aarch64 root@192.168.1.1:/opt/sbin/xkeen-panel
	scp config.yaml root@192.168.1.1:/opt/etc/xkeen-panel/config.yaml

clean:
	rm -rf build/
	rm -rf frontend/dist
	rm -rf frontend/node_modules
