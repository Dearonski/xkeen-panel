#!/bin/sh
# Installs XKeen Panel on a Keenetic router
# Usage: sh install.sh [architecture]
# Architectures: aarch64 (default), mips, mipsel

set -e

REPO="Dearonski/xkeen-panel"
INSTALL_DIR="/opt/etc/xkeen-panel"
BIN_PATH="/opt/sbin/xkeen-panel"
INIT_SCRIPT="/opt/etc/init.d/S99xkeen-panel"

# Detect the architecture
ARCH="${1:-$(uname -m)}"
case "$ARCH" in
    aarch64|arm64) ARCH="aarch64" ;;
    mips)          ARCH="mips" ;;
    mipsel|mipsle) ARCH="mipsel" ;;
    *)
        echo "Неизвестная архитектура: $ARCH"
        echo "Использование: sh install.sh [aarch64|mips|mipsel]"
        exit 1
        ;;
esac

echo "=== XKeen Panel — установка ==="
echo "Архитектура: $ARCH"

# Check dependencies
if ! command -v curl >/dev/null 2>&1; then
    echo "Ошибка: curl не найден. Установите: opkg install curl"
    exit 1
fi

# Download the latest release
echo "Скачивание бинарника..."
DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" \
    | grep "browser_download_url.*$ARCH" \
    | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Ошибка: не удалось найти релиз для архитектуры $ARCH"
    echo "Проверьте: https://github.com/$REPO/releases"
    exit 1
fi

# Stop the running instance
if [ -f "$INIT_SCRIPT" ]; then
    echo "Остановка текущей версии..."
    "$INIT_SCRIPT" stop 2>/dev/null || true
fi

curl -L -o "$BIN_PATH" "$DOWNLOAD_URL"
chmod +x "$BIN_PATH"
echo "Бинарник: $BIN_PATH"

# Create directories
mkdir -p "$INSTALL_DIR/data"

# Config (never overwrite an existing one)
if [ ! -f "$INSTALL_DIR/config.yaml" ]; then
    cat > "$INSTALL_DIR/config.yaml" << 'YAML'
# Web panel port
port: 3000

# Data directory (user.json, subscription.json)
data_dir: /opt/etc/xkeen-panel/data

# === XKeen/Xray paths ===
# The XKeen layout (S05xkeen/S24xray), the active core and the proxying mode
# are detected automatically — set the paths below only for a non-standard
# install.
xkeen_path: /opt/sbin/xkeen
outbounds_file: /opt/etc/xray/configs/04_outbounds.json

# === Watchdog ===
check_interval: 120
check_url: https://www.google.com
max_fails: 3

# Log file
log_file: /opt/var/log/xkeen-panel.log

# Hour (router time) at which the panel installs its own updates.
# Automatic updates themselves are switched on and off in the panel.
auto_update_hour: 4

# Balancer pool size (lowest latency, without avoided countries)
pool_max_nodes: 10

# Probe real services through the active node (catches IPs banned by CDNs)
health_check_every: 5
health_fail_threshold: 2
health_quorum: 2
YAML
    echo "Конфиг: $INSTALL_DIR/config.yaml"
else
    echo "Конфиг уже существует, пропущен"
fi

# Create the log directory
mkdir -p /opt/var/log

# Init script
cat > "$INIT_SCRIPT" << 'INITSCRIPT'
#!/bin/sh

ENABLED=yes
PROCS="xkeen-panel"
ARGS="-config /opt/etc/xkeen-panel/config.yaml"
DESC="XKeen Panel"

PREARGS=""
. /opt/etc/init.d/rc.func
INITSCRIPT
chmod +x "$INIT_SCRIPT"
echo "Init-скрипт: $INIT_SCRIPT"

# Start
echo ""
echo "=== Установка завершена ==="
echo ""
echo "Запуск:  $INIT_SCRIPT start"
echo "Панель:  http://<IP роутера>:3000"
echo ""

"$INIT_SCRIPT" start
