# XKeen Panel

Веб-панель для управления [XKeen](https://github.com/Jenya-XKeen/XKeen)/Xray на роутерах Keenetic.

![dashboard](https://img.shields.io/badge/stack-Go%20%2B%20React-blue)

## Возможности

- **Управление подписками** — добавление URL, обновление списка серверов
- **Выбор сервера** — переключение активного сервера с автоматическим рестартом Xray
- **Проверка латенси** — стриминг пинга каждого сервера в реальном времени (SSE)
- **Watchdog** — автоматическая проверка соединения и переключение на следующий сервер при сбоях
- **Логи в реальном времени** — через Server-Sent Events, без поллинга
- **Аутентификация** — JWT + TOTP (двухфакторная)

## Установка на роутер

### Требования

- Keenetic с установленным [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- Установленный [XKeen](https://github.com/Jenya-XKeen/XKeen)
- Пакет `curl` (`opkg install curl`)

### Быстрая установка

Подключитесь к роутеру по SSH и выполните:

```sh
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh
```

Если архитектура не определилась автоматически:

```sh
# Для Keenetic Giga, Ultra, Peak и других ARM64
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh -s aarch64

# Для Keenetic с MIPS (старые модели)
curl -sL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh -s mipsel
```

### Ручная установка

```sh
# 1. Скачать бинарник (выберите свою архитектуру)
curl -L -o /opt/sbin/xkeen-panel \
  https://github.com/Dearonski/xkeen-panel/releases/latest/download/xkeen-panel-aarch64
chmod +x /opt/sbin/xkeen-panel

# 2. Создать директории
mkdir -p /opt/etc/xkeen-panel/data

# 3. Создать конфиг
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

# 4. Создать init-скрипт для автозапуска
cat > /opt/etc/init.d/S99xkeen-panel << 'EOF'
#!/bin/sh

PROCS="xkeen-panel"
ARGS="-config /opt/etc/xkeen-panel/config.yaml"
DESC="XKeen Panel"

PREARGS=""
. /opt/etc/init.d/rc.func
EOF
chmod +x /opt/etc/init.d/S99xkeen-panel

# 5. Запустить
/opt/etc/init.d/S99xkeen-panel start
```

### После установки

1. Откройте в браузере `http://<IP роутера>:3000`
2. Создайте учётную запись (логин + пароль)
3. Отсканируйте QR-код для TOTP (Google Authenticator, Aegis и т.д.)
4. Войдите с логином, паролем и кодом из приложения

## Управление

```sh
# Запуск / остановка / перезапуск
/opt/etc/init.d/S99xkeen-panel start
/opt/etc/init.d/S99xkeen-panel stop
/opt/etc/init.d/S99xkeen-panel restart

# Логи
tail -f /opt/var/log/xkeen-panel.log
```

## Обновление

```sh
# Остановить панель
/opt/etc/init.d/S99xkeen-panel stop

# Скачать новую версию
curl -L -o /opt/sbin/xkeen-panel \
  https://github.com/Dearonski/xkeen-panel/releases/latest/download/xkeen-panel-aarch64
chmod +x /opt/sbin/xkeen-panel

# Запустить
/opt/etc/init.d/S99xkeen-panel start
```

Или повторно запустить install.sh — он не перезаписывает конфиг.

## Конфигурация

Файл: `/opt/etc/xkeen-panel/config.yaml`

| Параметр | По умолчанию | Описание |
|----------|-------------|----------|
| `port` | `3000` | Порт веб-панели |
| `data_dir` | `/opt/etc/xkeen-panel/data` | Директория данных |
| `xkeen_path` | `/opt/sbin/xkeen` | Путь к бинарнику XKeen |
| `outbounds_file` | `/opt/etc/xray/configs/04_outbounds.json` | Конфиг outbounds Xray |
| `check_interval` | `120` | Интервал проверки watchdog (секунды) |
| `check_url` | `https://www.google.com` | URL для проверки соединения |
| `max_fails` | `3` | Сбоев подряд до переключения сервера |

## Сборка из исходников

```sh
# Зависимости
go 1.23+
node 20+

# Собрать для ARM64 (Keenetic Giga/Ultra/Peak)
make build-arm64

# Бинарник: build/xkeen-panel
```

## Стек

- **Backend:** Go, chi router, JWT, TOTP, SSE
- **Frontend:** React 19, Vite, Tailwind CSS 4, TanStack Query
- Фронтенд встроен в бинарник через `go:embed`
