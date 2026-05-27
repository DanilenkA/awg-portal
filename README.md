# awg-portal

> WireGuard Portal с нативной поддержкой AmneziaWG — обфускация DPI без изменения логики портала.

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)
[![Release](https://img.shields.io/badge/release-v1.0.0-blue.svg)](https://github.com/DanilenkA/awg-portal/releases)

## Что это

Форк [h44z/wg-portal](https://github.com/h44z/wg-portal) с интегрированной поддержкой [AmneziaWG](https://github.com/amnezia-vpn/amneziawg-go) — протокола WireGuard с обфускацией для обхода DPI.

**Ключевое отличие от vanilla wg-portal:** интерфейсы управляются через `amneziawg-go` (userspace-демон) вместо ядерного модуля WireGuard. Параметры обфускации (Jc, Jmin, Jmax, S1-S4, H1-H4) прозрачно применяются на уровне интерфейса — все пиры наследуют их автоматически.

## Архитектура

```
wg-portal UI/API
 │
 ├─ wgctrl.ConfigureDevice()      ← стандартный вызов (Jipok/wgctrl-go)
 │   └─ UAPI → amneziawg-go       ← AWG-параметры встроены в Config
 │
 ├─ lowlevel.StartAWGProcess()    ← управление процессом (--foreground)
 └─ lowlevel.StopAWGProcess()     ← чистый останов
```

**Три точки интеграции, ноль изменений в бизнес-логике.**

- `awg_mode: auto` в конфиге — пробует amneziawg-go, fallback на kernel WG
- Параметры обфускации на уровне интерфейса — пиры наследуют автоматически
- Multi-interface: каждый интерфейс — отдельный процесс amneziawg-go
- wg-portal остаётся владельцем lifecycle интерфейсов

## Быстрый старт

```bash
# 1. Собрать amneziawg-go (требуется Go 1.21+)
git clone https://github.com/amnezia-vpn/amneziawg-go
cd amneziawg-go
CGO_ENABLED=0 go build -o /usr/local/bin/amneziawg-go -tags netgo .

# 2. Скачать и установить wg-portal
wget https://github.com/DanilenkA/awg-portal/releases/download/v1.0.0/wg-portal-amd64
sudo install -m 0755 wg-portal-amd64 /usr/local/bin/wg-portal

# 3. Создать конфиг
sudo mkdir -p /opt/wg-portal
sudo cp config.yml.sample /opt/wg-portal/config.yml
# Отредактировать: admin_user, admin_password, listening_address

# 4. Запустить
sudo systemctl start wg-portal
sudo journalctl -u wg-portal -f
```

## Конфигурация AWG

```yaml
# config.yml
backend:
  awg_mode: auto   # auto | always | never
```

| Режим | Поведение |
|-------|-----------|
| `auto` | Пробует amneziawg-go; если не найден → kernel WG |
| `always` | Требует amneziawg-go; ошибка при отсутствии |
| `never` | Только kernel WG; AWG-параметры игнорируются |

AWG-параметры (Jc, Jmin, Jmax, S1-S4, H1-H4) генерируются автоматически при создании интерфейса с `awg_enabled: true`. Полная документация — [wgportal.org](https://wgportal.org).

## Сборка из исходников

```bash
git clone --recurse-submodules https://github.com/DanilenkA/awg-portal
cd awg-portal

# Сборка (требуется Go 1.25+ и Node.js 22+)
make dist

# Результат:
ls -lh dist/
# wg-portal-amd64   41 MB  статический ELF, stripped
# install.sh               скрипт установки
# VERSION                  версия сборки
```

Бинарник статический — работает на любом Linux x86_64 (Ubuntu 22.04/24.04, Debian 12, RHEL 9).

## Обновление upstream

См. [UPSTREAM.md](UPSTREAM.md).

## Лицензия

MIT. Основано на [h44z/wg-portal](https://github.com/h44z/wg-portal) (MIT) и [Jipok/wgctrl-go](https://github.com/Jipok/wgctrl-go) (MIT).

## Troubleshooting

### amneziawg-go не найден
```bash
which amneziawg-go || echo "Установите: см. Быстрый старт → шаг 1"
```

### Крах amneziawg-go
Перезапустите wg-portal — `RestoreInterfaceState` пересоздаст интерфейсы:
```bash
systemctl restart wg-portal
```

### Проверка обфускации
```bash
# Сокет AWG
ls /var/run/amneziawg/

# AWG-параметры в БД
sqlite3 /opt/wg-portal/data/wg_portal.db \
  "SELECT identifier, awg_enabled, awg_jc, awg_h1 FROM interfaces"

# Первый байт трафика ≠ 0x01 (WireGuard initiation)
tcpdump -i eth0 udp port 51820 -c 3 -X | grep "0x0000"
```
