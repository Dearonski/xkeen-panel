.PHONY: dev dev-backend dev-frontend build-frontend build-arm64 build-local clean deploy

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
		-ldflags="-s -w" \
		-o build/xkeen-panel \
		.

build-local: build-frontend
	go build -o build/xkeen-panel .

compress: build-arm64
	upx --best build/xkeen-panel

# === Деплой ===

# Диск роутера = /opt/ на роутере
ROUTER_DISK ?= /Volumes/2bdb2bac-02b0-480a-9d7d-3affdea7b5ee

deploy-disk: build-arm64
	mkdir -p $(ROUTER_DISK)/etc/xkeen-panel/data
	cp build/xkeen-panel $(ROUTER_DISK)/sbin/xkeen-panel
	@test -f $(ROUTER_DISK)/etc/xkeen-panel/config.yaml || cp config.yaml $(ROUTER_DISK)/etc/xkeen-panel/config.yaml
	@echo "Deployed to router disk"

deploy-ssh: build-arm64
	scp build/xkeen-panel root@192.168.1.1:/opt/sbin/xkeen-panel
	scp config.yaml root@192.168.1.1:/opt/etc/xkeen-panel/config.yaml

clean:
	rm -rf build/
	rm -rf frontend/dist
	rm -rf frontend/node_modules
