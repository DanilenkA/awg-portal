# AWG-PORTAL

[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](https://opensource.org/licenses/MIT)
![GitHub last commit](https://img.shields.io/github/last-commit/DanilenkA/awg-portal/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/DanilenkA/awg-portal)](https://goreportcard.com/report/github.com/DanilenkA/awg-portal)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/DanilenkA/awg-portal)

## Introduction

**AWG-PORTAL** — это форк [h44z/wg-portal](https://github.com/h44z/wg-portal) с полной поддержкой [AmneziaWG](https://github.com/amnezia-vpn/amneziawg-go) — протокола, устойчивого к DPI и блокировкам.

Портал предоставляет веб-интерфейс для управления VPN-серверами на базе **WireGuard** и **AmneziaWG**. Поддерживает создание/удаление пиров, генерацию конфигов, QR-коды, email-рассылку, мониторинг и REST API.

Обфускация AmneziaWG настраивается через веб-интерфейс для каждого интерфейса отдельно — параметры автоматически передаются в конфигурации клиентов.

## Возможности

* Полная поддержка **WireGuard** и **AmneziaWG** (amneziawg-go v2.x / awg)
* Обфускация трафика — защита от DPI (Deep Packet Inspection)
* Автовыбор AWG-параметров с возможностью ручной настройки
* Самодостаточный бинарник — всё в одном файле
* Адаптивный веб-интерфейс на Vue.js с тёмной темой и мультиязычностью
* Автовыбор IP из пула сети при создании пира
* QR-код для удобной настройки мобильных клиентов
* Отправка конфига по email
* Включение / отключение пиров без прерывания соединений
* Генерация wg-quick конфигов (`wgX.conf`)
* Аутентификация (БД, OAuth, LDAP), поддержка Passkey
* IPv6 готовность
* Docker-ready
* Работа с существующими WireGuard-интерфейсами
* Поддержка нескольких интерфейсов и бекендов (wgctl, MikroTik, pfSense)
* Управление маршрутизацией и DNS (как wg-quick)
* Prometheus-метрики для мониторинга
* REST API для управления и деплоя клиентов
* Webhook для кастомных действий

## Отличия от оригинала (h44z/wg-portal)

| Возможность | h44z/wg-portal | AWG-PORTAL |
|---|---|---|
| WireGuard | ✅ | ✅ |
| AmneziaWG (обфускация) | ❌ | ✅ |
| amneziawg-go | ❌ | ✅ (v2.x) |
| AWG-параметры в API | ❌ | ✅ |
| UAPI для AWG | ❌ | ✅ |
| AWG-бейдж в интерфейсе | ❌ | ✅ |
| SVG-логотип | ❌ | ✅ |
| PresharedKey/PrivateKey автогенерация в API | ❌ | ✅ |

## Быстрый старт

### Docker (рекомендовано)

```bash
# 1. Скачать docker-compose.yml
curl -LO https://raw.githubusercontent.com/DanilenkA/awg-portal/main/docker-compose.yml

# 2. Отредактировать docker-compose.yml — поменять:
#    - WG_PORTAL_CORE_ADMIN_USER
#    - WG_PORTAL_CORE_ADMIN_PASSWORD
#    - WG_PORTAL_WEB_EXTERNAL_URL

# 3. Создать директории для данных
mkdir -p data config

# 4. Запустить
docker compose up -d

# 5. Открыть браузер: http://<сервер>:8888
```

Контейнер использует `network_mode: host` — портал управляет сетевыми интерфейсами непосредственно на хосте. Все порты (8888, 8787, 51820+) открываются на хосте.

### Бинарный релиз

```bash
# Скачать последний релиз
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/awg-portal-linux-amd64.tar.gz
tar xzf awg-portal-linux-amd64.tar.gz
sudo ./awg-portal --config config.yml
```

Пример конфига: [config.yml.sample](config.yml.sample)

Полная документация: [wgportal.org](https://wgportal.org) (оригинальная, функционал совместим).

## Docker

### Доступные образы

Образы публикуются на GitHub Container Registry:

```
ghcr.io/danilenka/awg-portal:latest     # последний стабильный
ghcr.io/danilenka/awg-portal:vX.X.X     # конкретная версия
```

### Системные требования

- **Docker Engine** 24+ (рекомендуется с поддержкой Compose v2)
- **Ядро Linux** с модулем `wireguard` (5.6+) или `amneziawg` (для AWG)
- Доступ к сетевым интерфейсам — контейнер требует `--cap-add=NET_ADMIN,SYS_MODULE` и `--network=host`

### Конфигурация через переменные окружения

Все параметры [config.yml.sample](config.yml.sample) можно задать через переменные окружения. Схема именования:

```
WG_PORTAL_<СЕКЦИЯ>_<ПАРАМЕТР>
```

Пример: `advanced.log_level` → `WG_PORTAL_ADVANCED_LOG_LEVEL`

Базовые переменные (обязательны к настройке):

| Переменная | Описание | Пример |
|---|---|---|
| `WG_PORTAL_CORE_ADMIN_USER` | Email администратора | `admin@example.com` |
| `WG_PORTAL_CORE_ADMIN_PASSWORD` | Пароль администратора | (смените!) |
| `WG_PORTAL_WEB_EXTERNAL_URL` | Внешний URL портала | `http://vpn.example.com:8888` |
| `WG_PORTAL_ADVANCED_CONFIG_STORAGE_PATH` | Путь для wg-конфигов | `/app/config` |

> **Важно:** `WG_PORTAL_ADVANCED_CONFIG_STORAGE_PATH` обязателен. Без него
> функция `save-config` падает с nil pointer.

### Монтируемые тома

| Том хоста | В контейнере | Назначение |
|---|---|---|
| `/etc/wireguard` | `/etc/wireguard` | WireGuard/AmneziaWG-конфиги интерфейсов |
| `./data` | `/app/data` | SQLite БД, логи, ключи |
| `./config` | `/app/config` | wg-конфиги (save-config) |

### docker-compose.yml (полный пример)

```yaml
services:
  awg-portal:
    image: ghcr.io/danilenka/awg-portal:latest
    container_name: awg-portal
    restart: unless-stopped
    network_mode: "host"
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    devices:
      - /dev/net/tun:/dev/net/tun  # Required for AWG userspace mode
    environment:
      - WG_PORTAL_CORE_ADMIN_USER=admin@example.com
      - WG_PORTAL_CORE_ADMIN_PASSWORD=CHANGE_ME_PLEASE
      - WG_PORTAL_CORE_RESTORE_STATE=true
      - WG_PORTAL_CORE_IMPORT_EXISTING=true
      - WG_PORTAL_WEB_EXTERNAL_URL=http://localhost:8888
      - WG_PORTAL_WEB_LISTENING_ADDRESS=:8888
      - WG_PORTAL_WEB_SITE_TITLE=AWG-PORTAL
      - WG_PORTAL_ADVANCED_LOG_LEVEL=info
      - WG_PORTAL_ADVANCED_START_LISTEN_PORT=51820
      - WG_PORTAL_ADVANCED_START_CIDR_V4=10.211.1.0/24
      - WG_PORTAL_ADVANCED_CONFIG_STORAGE_PATH=/app/config
      - WG_PORTAL_BACKEND_AWG_MODE=auto
      - WG_PORTAL_DATABASE_TYPE=sqlite
      - WG_PORTAL_DATABASE_DSN=/app/data/sqlite.db
    volumes:
      - /etc/wireguard:/etc/wireguard
      - ./data:/app/data
      - ./config:/app/config
```

Полный файл с комментариями — в [docker-compose.yml](docker-compose.yml) репозитория.

### Использование config.yml вместо переменных окружения

Если предпочитаете YAML-файл, смонтируйте его в `/app/config/config.yml`:

```yaml
services:
  awg-portal:
    image: ghcr.io/danilenka/awg-portal:latest
    network_mode: "host"
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    volumes:
      - ./config.yml:/app/config/config.yml
      - /etc/wireguard:/etc/wireguard
      - ./data:/app/data
```

### Сборка образа из исходников

```bash
# Требования: Docker 24+
git clone git@github.com:DanilenkA/awg-portal.git
cd awg-portal

# Собрать образ (теги: latest + версия из git describe)
make build-docker

# Мультиархитектурная сборка (amd64 + arm64) с пушами в ghcr.io
make build-docker-multiarch
```

### Особенности работы в Docker

1. **Физические интерфейсы:** Портал управляет WireGuard-интерфейсами на
   хосте через `wg set`/`ip link`. Это нормально — интерфейсы видны на хосте
   и сохраняются после перезапуска контейнера (если `restore_state=true`).

2. **save-config:** Для сохранения wg-конфигов на диск требуется
   `WG_PORTAL_ADVANCED_CONFIG_STORAGE_PATH`. Без него save-config не работает.

3. **Маршрутизация:** Портал использует `wg set`, а не `wg-quick`. Если нужна
   автоматическая настройка маршрутов (fwmark, таблица маршрутизации),
   добавьте PostUp-скрипты или используйте `wg-quick` на хосте.

4. **PresharedKey:** При создании пира через REST API без указания
   `PrivateKey` и `PresharedKey` — портал сгенерирует их автоматически.
   Это исправлено в AWG-PORTAL (в оригинале h44z ключи оставались пустыми).

5. **AmneziaWG (обфускация):** Для работы AWG требуется бинарник
   `amneziawg-go`. В Docker-образ AWG-PORTAL он **встроен** —
   дополнительная установка не требуется.

6. **TUN-устройство:** Для работы AWG (userspace-режим amneziawg-go)
   необходимо монтировать `/dev/net/tun` в контейнер и добавить
   `--privileged` или устройство в devices:
   ```yaml
   devices:
     - /dev/net/tun:/dev/net/tun
   ```

## Установка AmneziaWG

AWG-PORTAL автоматически управляет процессом `amneziawg-go`. Если бинарный бандл содержит `amneziawg-go`, портал запустит его в фоне. В режиме `awg_mode: auto` портал сам определяет, какой протокол использовать, на основе настроек интерфейса.

Подробнее: [AmneziaWG](https://docs.amnezia.org/ru/documentation/amnezia-wg/)

## Сборка из исходников

```bash
# Требования: Go 1.25+, Node.js 20+, Docker 24+
git clone git@github.com:DanilenkA/awg-portal.git
cd awg-portal/wg-portal

# Docker-образ (включает amneziawg-go)
docker build --build-arg BUILD_VERSION=v1.3.0 -t ghcr.io/danilenka/awg-portal:v1.3.0 .

# Или бинарник (требуется Go на хосте)
CGO_ENABLED=0 go build -ldflags "-X github.com/DanilenkA/awg-portal/internal.Version=v1.3.0" -o awg-portal_x86-64 cmd/wg-portal/main.go
```

## Application stack

* [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — AWG-протокол
* [wgctrl-go](https://github.com/WireGuard/wgctrl-go) и [netlink](https://github.com/vishvananda/netlink) — управление интерфейсами
* [Bootstrap](https://getbootstrap.com/) — HTML-шаблоны
* [Vue.js](https://vuejs.org/) — фронтенд
* [Vite](https://vite.dev/) — сборка фронтенда

## REST API

Портал предоставляет REST API для автоматизации. После запуска:

```bash
# Логин (получить JWT-токен)
curl -X POST http://localhost:8888/api/v0/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@example.com","password":"CHANGE_ME_PLEASE"}'

# Создать интерфейс (замените JWT)
curl -X POST http://localhost:8888/api/v0/interface/new \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"InterfaceName":"wg0","IpAddresses":["10.211.1.1/24"],"ListenPort":51820}'

# Создать пира
curl -X POST http://localhost:8888/api/v0/peer/iface/wg0/new \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"DisplayName":"client1","IPAddresses":["10.211.1.2/32"]}'

# Получить конфиг пира
curl -X GET http://localhost:8888/api/v0/peer/config/<peer-id> \
  -H "Authorization: Bearer <JWT>"
```

API v0 полностью совместимо с h44z/wg-portal. Документация:
[Swagger: /api/v0/docs/](https://wgportal.org/master/rest-api/)

## Лицензия

MIT License. [MIT](LICENSE.txt)

## Благодарности

Огромное спасибо [h44z](https://github.com/h44z) за оригинальный [WireGuard Portal](https://github.com/h44z/wg-portal).
