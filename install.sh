#!/bin/sh
# Installs XKeen Panel on a Keenetic router with Entware, and XKeen itself with --with-xkeen

REPO="Dearonski/xkeen-panel"
XKEEN_URL="https://github.com/jameszeroX/XKeen/releases/latest/download/xkeen.tar.gz"
MIRRORS="https://gh-proxy.com https://ghfast.top"

INSTALL_DIR="/opt/etc/xkeen-panel"
BIN_PATH="/opt/sbin/xkeen-panel"
INIT_SCRIPT="/opt/etc/init.d/S99xkeen-panel"
XKEEN_BIN="/opt/sbin/xkeen"

WITH_XKEEN=0
WITH_MIHOMO=0
CHECK_ONLY=0
ARCH=""

green="\033[92m"
red="\033[91m"
yellow="\033[93m"
reset="\033[0m"
if [ ! -t 1 ]; then
    green="" red="" yellow="" reset=""
fi

ok() { printf "  ${green}✓${reset} %s\n" "$*"; }
warn() { printf "  ${yellow}!${reset} %s\n" "$*"; }
bad() { printf "  ${red}✗${reset} %s\n" "$*"; }
fail() {
    printf "\n${red}Ошибка${reset}: %s\n" "$*"
    exit 1
}

usage() {
    cat <<'EOF'
Установка XKeen Panel на роутер Keenetic с Entware.

  sh install.sh                 поставить или обновить панель
  sh install.sh --with-xkeen    поставить XKeen (если его нет) и панель
  sh install.sh --check         только проверить роутер, ничего не меняя

  --mihomo                      вместе с XKeen поставить и ядро Mihomo
  aarch64 | mipsel              указать архитектуру вручную
EOF
}

for arg in "$@"; do
    case "$arg" in
        --with-xkeen) WITH_XKEEN=1 ;;
        --mihomo) WITH_MIHOMO=1 ;;
        --check) CHECK_ONLY=1 ;;
        -h | --help)
            usage
            exit 0
            ;;
        aarch64 | arm64) ARCH="aarch64" ;;
        mipsel | mipsle) ARCH="mipsel" ;;
        *)
            usage
            fail "неизвестный параметр: $arg"
            ;;
    esac
done

# GitHub is often slow or blocked from Russian ISPs; the mirrors proxy release downloads
fetch() {
    url="$1"
    out="$2"
    curl -fL --connect-timeout 10 -m 600 -o "$out" "$url" 2>/dev/null && return 0
    for mirror in $MIRRORS; do
        curl -fL --connect-timeout 10 -m 600 -o "$out" "$mirror/$url" 2>/dev/null && return 0
    done
    return 1
}

detect_arch() {
    [ -n "$ARCH" ] && return 0
    # uname -m says "mips" for both endiannesses; opkg knows which one Entware runs
    opkg_arch=$(opkg print-architecture 2>/dev/null | awk '$2 != "all" && $2 != "noarch" {a=$2} END {print a}')
    case "$opkg_arch$(uname -m)" in
        aarch64* | *aarch64 | *arm64) ARCH="aarch64" ;;
        mipsel*) ARCH="mipsel" ;;
        mips*) fail "big-endian MIPS не поддерживается: сборки есть только для aarch64 и mipsel" ;;
        *) fail "не удалось определить архитектуру ($opkg_arch $(uname -m)). Укажите её вручную: aarch64 или mipsel" ;;
    esac
}

xkeen_installed() {
    [ -x "$XKEEN_BIN" ] && [ -d /opt/sbin/.xkeen ]
}

xkeen_version() {
    sed -n 's/^xkeen_current_version="\{0,1\}\([^"]*\)"\{0,1\}$/\1/p' /opt/sbin/.xkeen/01_info/01_info_variable.sh 2>/dev/null | head -n 1
}

PREFLIGHT_FAILED=0

preflight() {
    printf "\nПроверка роутера\n"

    if [ "$(id -u)" = "0" ]; then ok "запуск от root"; else bad "нужен root — подключитесь к Entware по SSH как root"; PREFLIGHT_FAILED=1; fi

    if command -v opkg >/dev/null 2>&1 && [ -d /opt/etc ]; then
        ok "Entware установлен"
    else
        bad "Entware не найден — установите его на USB-накопитель через веб-интерфейс Keenetic"
        PREFLIGHT_FAILED=1
    fi

    if command -v ndmc >/dev/null 2>&1; then ok "роутер Keenetic"; else bad "это не Keenetic: нет ndmc"; PREFLIGHT_FAILED=1; fi

    ok "архитектура $ARCH"

    components=$(curl -s -m 5 http://127.0.0.1:79/rci/show/version 2>/dev/null | sed -n 's/.*"components": *"\([^"]*\)".*/\1/p')
    if [ -z "$components" ]; then
        warn "не удалось прочитать список компонентов Keenetic — проверьте их вручную"
    else
        case ",$components," in
            *,opkg-kmod-netfilter,*) ok "компонент «Модули ядра подсистемы Netfilter»" ;;
            *)
                bad "нет компонента «Модули ядра подсистемы Netfilter» — включите его в «Общих настройках» Keenetic"
                PREFLIGHT_FAILED=1
                ;;
        esac
        case ",$components," in
            *,ip6,*) ok "компонент «Протокол IPv6»" ;;
            *) warn "нет компонента «Протокол IPv6» — XKeen будет работать только с IPv4" ;;
        esac
    fi

    need_kb=20000
    [ "$WITH_XKEEN" = 1 ] && ! xkeen_installed && need_kb=200000
    free_kb=$(df -k /opt 2>/dev/null | awk 'NR == 2 {print $4}')
    if [ -n "$free_kb" ] && [ "$free_kb" -ge "$need_kb" ]; then
        ok "свободно $((free_kb / 1024)) МБ на /opt"
    else
        bad "на /opt мало места: нужно хотя бы $((need_kb / 1024)) МБ"
        PREFLIGHT_FAILED=1
    fi

    if curl -sI -m 8 https://github.com >/dev/null 2>&1; then
        ok "GitHub доступен"
    else
        mirror_ok=0
        for mirror in $MIRRORS; do
            curl -sI -m 8 "$mirror" >/dev/null 2>&1 && mirror_ok=1 && break
        done
        if [ "$mirror_ok" = 1 ]; then
            warn "GitHub недоступен напрямую — скачаю через зеркало"
        else
            bad "нет доступа ни к GitHub, ни к зеркалам"
            PREFLIGHT_FAILED=1
        fi
    fi

    if xkeen_installed; then
        ok "XKeen $(xkeen_version) уже установлен"
    elif [ "$WITH_XKEEN" = 1 ]; then
        ok "XKeen будет установлен"
    else
        warn "XKeen не установлен — добавьте --with-xkeen, чтобы поставить его вместе с панелью"
    fi
}

install_xkeen() {
    printf "\nУстановка XKeen\n"
    work=$(mktemp -d /tmp/xkeen-install.XXXXXX) || fail "не удалось создать временную папку"

    fetch "$XKEEN_URL" "$work/xkeen.tar.gz" || fail "не удалось скачать XKeen ни с GitHub, ни через зеркала"
    mkdir -p "$work/stage"
    tar -xzf "$work/xkeen.tar.gz" -C "$work/stage" 2>/dev/null || fail "архив XKeen повреждён"
    [ -f "$work/stage/xkeen" ] && [ -d "$work/stage/_xkeen" ] || fail "в архиве XKeen нет ожидаемых файлов"

    rm -rf /opt/sbin/.xkeen
    mv "$work/stage/_xkeen" /opt/sbin/.xkeen || fail "не удалось установить модули XKeen"
    mv "$work/stage/xkeen" "$XKEEN_BIN" && chmod +x "$XKEEN_BIN" || fail "не удалось установить XKeen"
    rm -rf "$work"
    ok "XKeen скачан"

    cores="xray"
    [ "$WITH_MIHOMO" = 1 ] && cores="all"

    # Autostart stays off until the panel writes a working config; </dev/null stops XKeen reading this piped script
    printf "  Ставлю ядро и геобазы — это займёт несколько минут\n\n"
    "$XKEEN_BIN" -i auto cores="$cores" geo=on geoipset=off cron=on autostart=off </dev/null ||
        fail "установка XKeen завершилась с ошибкой — подробности выше"
    xkeen_installed || fail "XKeen не установился"
    ok "XKeen $(xkeen_version) установлен"
}

verify_sha() {
    file="$1"
    name="$2"
    sums="$3"
    if ! command -v sha256sum >/dev/null 2>&1; then
        warn "нет sha256sum — контрольная сумма не проверена"
        return 0
    fi
    expected=$(awk -v n="$name" '$2 == n {print $1}' "$sums")
    [ -n "$expected" ] || fail "в SHA256SUMS нет $name"
    actual=$(sha256sum "$file" | awk '{print $1}')
    [ "$expected" = "$actual" ] || fail "контрольная сумма $name не совпала — файл повреждён или подменён"
}

write_config() {
    mkdir -p "$INSTALL_DIR/data" /opt/var/log
    if [ -f "$INSTALL_DIR/config.yaml" ]; then
        ok "конфиг уже есть, оставляю как есть"
        return 0
    fi
    cat >"$INSTALL_DIR/config.yaml" <<'YAML'
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
    ok "конфиг $INSTALL_DIR/config.yaml"
}

write_init() {
    cat >"$INIT_SCRIPT" <<'INITSCRIPT'
#!/bin/sh

ENABLED=yes
PROCS="xkeen-panel"
ARGS="-config /opt/etc/xkeen-panel/config.yaml"
DESC="XKeen Panel"

PREARGS=""
. /opt/etc/init.d/rc.func
INITSCRIPT
    chmod +x "$INIT_SCRIPT"
}

install_panel() {
    printf "\nУстановка панели\n"
    work=$(mktemp -d /tmp/xkeen-panel.XXXXXX) || fail "не удалось создать временную папку"
    asset="xkeen-panel-$ARCH"
    base="https://github.com/$REPO/releases/latest/download"

    fetch "$base/$asset" "$work/$asset" || fail "не удалось скачать $asset ни с GitHub, ни через зеркала"
    fetch "$base/SHA256SUMS" "$work/SHA256SUMS" || fail "не удалось скачать SHA256SUMS"
    verify_sha "$work/$asset" "$asset" "$work/SHA256SUMS"
    chmod +x "$work/$asset"
    version=$("$work/$asset" -version 2>/dev/null) || fail "скачанная панель не запускается на этом роутере"
    ok "панель $version скачана и проверена"

    [ -x "$INIT_SCRIPT" ] && "$INIT_SCRIPT" stop >/dev/null 2>&1
    [ -f "$BIN_PATH" ] && cp "$BIN_PATH" "$BIN_PATH.bak"
    mv "$work/$asset" "$BIN_PATH" || fail "не удалось заменить $BIN_PATH"
    rm -rf "$work"

    write_config
    write_init
    "$INIT_SCRIPT" start >/dev/null 2>&1

    i=0
    while [ $i -lt 20 ]; do
        if curl -s -o /dev/null -m 2 http://127.0.0.1:3000/api/auth/status; then
            ok "панель запущена"
            return 0
        fi
        i=$((i + 1))
        sleep 1
    done
    warn "панель не ответила за 20 секунд — посмотрите /opt/var/log/xkeen-panel.log"
}

printf "=== XKeen Panel — установка ===\n"
detect_arch
preflight

if [ "$PREFLIGHT_FAILED" = 1 ]; then
    fail "роутер не готов — исправьте пункты с ✗ и запустите снова"
fi
if [ "$CHECK_ONLY" = 1 ]; then
    printf "\nРоутер готов к установке.\n"
    exit 0
fi

if [ "$WITH_XKEEN" = 1 ] && ! xkeen_installed; then
    install_xkeen
fi

install_panel

lan_ip=$(ip -4 -o addr show br0 2>/dev/null | awk '{sub(/\/.*/, "", $4); print $4; exit}')
printf "\n${green}Готово.${reset} Откройте http://%s:3000\n" "${lan_ip:-<IP роутера>}"
if [ ! -f "$INSTALL_DIR/data/user.json" ]; then
    printf "Создайте аккаунт — мастер настройки проведёт через подписку, устройства и запуск.\n"
fi
