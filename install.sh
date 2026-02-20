#!/bin/sh
# Установка XKeen Panel на Keenetic роутер
# Запуск: sh install.sh [архитектура]
# Архитектуры: aarch64 (по умолчанию), mips, mipsel

set -e

REPO="Dearonski/xkeen-panel"
INSTALL_DIR="/opt/etc/xkeen-panel"
BIN_PATH="/opt/sbin/xkeen-panel"
INIT_SCRIPT="/opt/etc/init.d/S99xkeen-panel"

# Определить архитектуру
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

# Проверить зависимости
if ! command -v curl >/dev/null 2>&1; then
    echo "Ошибка: curl не найден. Установите: opkg install curl"
    exit 1
fi

# Скачать последний релиз
echo "Скачивание бинарника..."
DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" \
    | grep "browser_download_url.*$ARCH" \
    | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Ошибка: не удалось найти релиз для архитектуры $ARCH"
    echo "Проверьте: https://github.com/$REPO/releases"
    exit 1
fi

# Остановить если запущен
if [ -f "$INIT_SCRIPT" ]; then
    echo "Остановка текущей версии..."
    "$INIT_SCRIPT" stop 2>/dev/null || true
fi

curl -L -o "$BIN_PATH" "$DOWNLOAD_URL"
chmod +x "$BIN_PATH"
echo "Бинарник: $BIN_PATH"

# Создать директории
mkdir -p "$INSTALL_DIR/data"

# Конфиг (не перезаписывать существующий)
if [ ! -f "$INSTALL_DIR/config.yaml" ]; then
    cat > "$INSTALL_DIR/config.yaml" << 'YAML'
# Порт веб-панели
port: 3000

# Папка для данных (user.json, subscription.json)
data_dir: /opt/etc/xkeen-panel/data

# === Пути XKeen/Xray ===
xkeen_path: /opt/sbin/xkeen
outbounds_file: /opt/etc/xray/configs/04_outbounds.json
init_script: /opt/etc/init.d/S24xray

# === Watchdog ===
check_interval: 120
check_url: https://www.google.com
max_fails: 3

# Лог-файл
log_file: /opt/var/log/xkeen-panel.log
YAML
    echo "Конфиг: $INSTALL_DIR/config.yaml"
else
    echo "Конфиг уже существует, пропущен"
fi

# Создать директорию для логов
mkdir -p /opt/var/log

# Init-скрипт
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

# Запуск
echo ""
echo "=== Установка завершена ==="
echo ""
echo "Запуск:  $INIT_SCRIPT start"
echo "Панель:  http://<IP роутера>:3000"
echo ""

"$INIT_SCRIPT" start
