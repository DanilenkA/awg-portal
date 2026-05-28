# awg-portal

> Web UI для WireGuard и AmneziaWG — обфускация DPI, управление интерфейсами и пирами.

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)
[![Release](https://img.shields.io/badge/release-v1.2.1fix-blue.svg)](https://github.com/DanilenkA/awg-portal/releases)

## Что это

Форк [h44z/wg-portal](https://github.com/h44z/wg-portal) с интегрированной поддержкой [AmneziaWG](https://github.com/amnezia-vpn/amneziawg-go) — протокола WireGuard с обфускацией для обхода DPI.

**Ключевые отличия:**
- Выбор протокола (WireGuard / AmneziaWG) при создании интерфейса
- Управление через `amneziawg-go` (userspace) или kernel WG
- Параметры обфускации генерируются автоматически
- Multi-interface: каждый интерфейс — отдельный процесс amneziawg-go
- Прямое управление AWG UAPI (в обход wgctrl-go) для пиров
- **v1.2.1fix:** исправлена передача AWG-параметров обфускации через API-модели пиров (Jc, Jmin, Jmax, S1–S4, H1–H4)

## История версий / Changelog

### v1.2.1fix (текущая)
- **Исправлено:** AWG-параметры (Jc, Jmin, Jmax, S1–S4, H1–H4) теперь корректно передаются через API-модели пиров.
  - Раньше: `PreparePeer()` генерировал AWG-параметры в `domain.Peer`, но `NewPeer()` (domain→model) их терял. При создании пира через UI AWG-параметры не попадали в БД.
  - Теперь: AWG-поля добавлены в model.Peer для v0 и v1 API, в конвертеры NewPeer/NewDomainPeer и во frontend model.
- **Исправлено:** В шаблоны конфигурации `wg_peer.tpl` и `wg_interface.tpl` добавлены пропущенные `S3` и `S4`.
- **Добавлено:** Прямое управление AWG UAPI для пиров (методы `SetAWGPeer`/`RemoveAWGPeer` в обход wgctrl-go).
- **Добавлено:** AWG stale socket recovery — автоматическое обнаружение и восстановление мёртвых сокетов `amneziawg-go`.

### v1.2.1
- AWG UAPI peer management (bypass wgctrl-go parse issues)
- FwMark Invert: false
- Protocol selector in UI (WG / AWG)
- AWG stale socket recovery

### v1.2.0
- AWG socket recovery
- Protocol selector in UI
- Rename to awg-portal

## Архитектура

```
awg-portal UI/API
 │
 ├─ Kernel WG (netlink)            ← awg_mode: never
 │
 ├─ wgctrl.ConfigureDevice()       ← awg_mode: auto
 │   └─ UAPI → amneziawg-go        ← AWG-параметры в Config
 │
 ├─ lowlevel.StartAWGProcess()     ← запуск amneziawg-go --foreground
 └─ lowlevel.StopAWGProcess()      ← чистый останов процесса

Прямое управление AWG UAPI (v1.2.1+):
 ├─ SetAWGPeer()                   ← добавление/обновление пира
 └─ RemoveAWGPeer()                ← удаление пира

API-модели пиров (v1.2.1fix):
 domain.PeerInterfaceConfig
   ├─ AWGEnabled bool
   ├─ AWGJc, AWGJmin, AWGJmax int
   ├─ AWGS1, AWGS2, AWGS3, AWGS4 int
   └─ AWGH1, AWGH2, AWGH3, AWGH4 uint32
```

### Выбор протокола

При создании интерфейса через веб-интерфейс можно выбрать:

| Протокол | Бэкенд | Когда использовать |
|----------|--------|-------------------|
| WireGuard | kernel WG (`awg_mode: never`) | Стабильность, производительность |
| AmneziaWG | `amneziawg-go` userspace | Обход DPI, обфускация трафика |

### AWG stale socket recovery

Если процесс `amneziawg-go` завершился, а сокет остался:
1. При следующем обращении к интерфейсу портал обнаруживает мёртвый сокет (`net.DialTimeout`)
2. Убивает orphan-процесс, удаляет файл сокета
3. Перезапускает `amneziawg-go --foreground`

## Быстрый старт

```bash
# 1. Скачать и распаковать релиз
wget https://github.com/DanilenkA/awg-portal/releases/download/v1.2.1fix/awg-portal-v1.2.1fix-bundle.tar.gz
tar xzf awg-portal-v1.2.1fix-bundle.tar.gz
cd awg-portal-v1.2.1fix/

# 2. Установить (от root)
sudo bash install.sh

# 3. Отредактировать конфиг
sudo nano /opt/awg-portal/config.yml
# Обязательно: admin_user, admin_password

# 4. Запустить
sudo systemctl enable --now awg-portal
sudo journalctl -u awg-portal -f
```

# 2. Установить (от root)
sudo bash install.sh

# 3. Отредактировать конфиг
sudo nano /opt/awg-portal/config.yml
# Обязательно: admin_user, admin_password

# 4. Запустить
sudo systemctl enable --now awg-portal
sudo journalctl -u awg-portal -f
```

## Конфигурация

```yaml
# /opt/awg-portal/config.yml
core:
  admin_user: admin@example.com
  admin_password: your-strong-password

backend:
  awg_mode: auto   # auto | always | never
```

| Параметр | Описание |
|----------|----------|
| `core.admin_user` | Email администратора |
| `core.admin_password` | Пароль (мин. 16 символов, или переопределить через `auth.min_password_length`) |
| `backend.awg_mode` | `auto` — amneziawg-go с fallback на kernel WG; `always` — только AWG; `never` — только kernel WG |
| `web.listening_address` | Адрес веб-интерфейса (по умолчанию `:8888`) |
| `database.type` | `sqlite` (по умолчанию) |

## Сборка из исходников

```bash
git clone --recurse-submodules https://github.com/DanilenkA/awg-portal
cd awg-portal

# Сборка (требуется Go 1.21+ и Node.js 22+)
make dist

# Результат:
ls -lh dist/
# awg-portal_x86-64  42 MB  статический ELF, stripped
# amneziawg-go       3.3 MB статический ELF, stripped
# install.sh                скрипт установки
# config.yml.sample         пример конфига
# README.md
# VERSION                   версия сборки
```

Бинарник статический — работает на любом Linux x86_64 (Ubuntu 22.04+, Debian 12, RHEL 9).

## Troubleshooting

### AWG socket connection refused
```bash
# Перезапуск сервиса восстановит AWG-процессы
systemctl restart awg-portal
```

### AWG-параметры не передаются пиру
Проверить версию — должно быть **v1.2.1fix** или новее. В версии v1.2.0/v1.2.1 AWG-параметры терялись при создании пира через API.

```bash
# Проверить версию
/usr/local/bin/awg-portal --version 2>&1 | head -1
# или через лог: grep "version" /var/log/awg-portal.log
```

### Kernel WG не отвечает на handshake
```bash
# Проверить, не мешают ли fwmark-правила от AWG-интерфейсов
ip rule show | grep fwmark

# Удалить лишние правила (если мешают):
# ip rule del priority N
```

### Проверка AWG UAPI
```bash
# Подключиться к сокету AWG-интерфейса
socat - UNIX-CONNECT:/var/run/amneziawg/<iface>.sock
# > get=1
```

### AWG-параметры в БД
```bash
sqlite3 /opt/awg-portal/data/wg_portal.db \
  "SELECT identifier, awg_enabled, awg_jc, awg_h1 FROM interfaces"
```

## Обновление upstream

См. [UPSTREAM.md](UPSTREAM.md).

## Лицензия

MIT. Основано на [h44z/wg-portal](https://github.com/h44z/wg-portal) (MIT) и [Jipok/wgctrl-go](https://github.com/Jipok/wgctrl-go) (MIT).
