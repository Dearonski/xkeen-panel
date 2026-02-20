# XKeen Panel

Web panel for managing [XKeen](https://github.com/Jenya-XKeen/XKeen)/Xray on Keenetic routers.

![dashboard](https://img.shields.io/badge/stack-Go%20%2B%20React-blue)

## Features

- **Subscription management** — add URL, refresh server list
- **Server selection** — switch active server with automatic Xray restart
- **Latency check** — real-time per-server ping streaming (SSE)
- **Watchdog** — automatic connection monitoring and failover to next server
- **Real-time logs** — via Server-Sent Events, no polling
- **Authentication** — JWT + TOTP (two-factor)

## Installation

### Requirements

- Keenetic router with [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- [XKeen](https://github.com/Jenya-XKeen/XKeen) installed
- `curl` package (`opkg install curl`)

### Quick install

Connect to your router via SSH and run:

```sh
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh
```

If architecture is not detected automatically:

```sh
# Keenetic Giga, Ultra, Peak and other ARM64 models
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh -s aarch64

# Keenetic with MIPS (older models)
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh -s mipsel
```

### Manual installation

```sh
# 1. Download binary (choose your architecture)
curl -L -o /opt/sbin/xkeen-panel \
  https://github.com/Dearonski/xkeen-panel/releases/latest/download/xkeen-panel-aarch64
chmod +x /opt/sbin/xkeen-panel

# 2. Create directories
mkdir -p /opt/etc/xkeen-panel/data

# 3. Create config
cat > /opt/etc/xkeen-panel/config.yaml << 'EOF'
port: 3000
data_dir: /opt/etc/xkeen-panel/data
xkeen_path: /opt/sbin/xkeen
outbounds_file: /opt/etc/xray/configs/04_outbounds.json
init_script: /opt/etc/init.d/S24xray
check_interval: 120
check_url: https://www.google.com
max_fails: 3
log_file: /opt/var/log/xkeen-panel.log
EOF

# 4. Create init script for autostart
cat > /opt/etc/init.d/S99xkeen-panel << 'EOF'
#!/bin/sh

PROCS="xkeen-panel"
ARGS="-config /opt/etc/xkeen-panel/config.yaml"
DESC="XKeen Panel"

PREARGS=""
. /opt/etc/init.d/rc.func
EOF
chmod +x /opt/etc/init.d/S99xkeen-panel

# 5. Start
/opt/etc/init.d/S99xkeen-panel start
```

### After installation

1. Open `http://<router IP>:3000` in your browser
2. Create an account (username + password)
3. Scan the QR code for TOTP (Google Authenticator, Aegis, etc.)
4. Log in with username, password and code from the app

## Usage

```sh
# Start / stop / restart
/opt/etc/init.d/S99xkeen-panel start
/opt/etc/init.d/S99xkeen-panel stop
/opt/etc/init.d/S99xkeen-panel restart

# Logs
tail -f /opt/var/log/xkeen-panel.log
```

## Updating

```sh
# Stop the panel
/opt/etc/init.d/S99xkeen-panel stop

# Download new version
curl -L -o /opt/sbin/xkeen-panel \
  https://github.com/Dearonski/xkeen-panel/releases/latest/download/xkeen-panel-aarch64
chmod +x /opt/sbin/xkeen-panel

# Start
/opt/etc/init.d/S99xkeen-panel start
```

Or re-run install.sh — it won't overwrite your config.

## Configuration

File: `/opt/etc/xkeen-panel/config.yaml`

| Parameter | Default | Description |
|-----------|---------|-------------|
| `port` | `3000` | Web panel port |
| `data_dir` | `/opt/etc/xkeen-panel/data` | Data directory |
| `xkeen_path` | `/opt/sbin/xkeen` | Path to XKeen binary |
| `outbounds_file` | `/opt/etc/xray/configs/04_outbounds.json` | Xray outbounds config |
| `check_interval` | `120` | Watchdog check interval (seconds) |
| `check_url` | `https://www.google.com` | URL for connection check |
| `max_fails` | `3` | Consecutive failures before server switch |

## Building from source

```sh
# Requirements
go 1.23+
node 20+

# Build for all architectures
make build-all

# Or specific target
make build-arm64   # Keenetic Giga/Ultra/Peak
make build-mipsel  # Keenetic older models

# Output: build/xkeen-panel-{aarch64,mipsel}
```

## Stack

- **Backend:** Go, chi router, JWT, TOTP, SSE
- **Frontend:** React 19, Vite, Tailwind CSS 4, TanStack Query
- Frontend is embedded into the binary via `go:embed`
