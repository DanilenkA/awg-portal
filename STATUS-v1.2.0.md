# awg-portal v1.2.0 — Статус

## Релиз

**GitHub:** https://github.com/DanilenkA/awg-portal/releases/tag/v1.2.0

**Ассеты:**
| Файл | Размер |
|------|--------|
| `awg-portal_x86-64` | 42 MB |
| `amneziawg-go` | 3.3 MB |
| `awg-portal-v1.2.0-bundle.tar.gz` | 17 MB |
| `install.sh` | скрипт установки |
| `config.yml.sample` | пример конфига |

**Ветка:** `feature/awg-backend`

---

## 1. Что сделано

### 1.1 Сборка релиза (`make dist`)
- Исправлен Makefile: `go build -C wg-portal`, `npm ci --include=dev`
- `make dist` работает end-to-end без ошибок
- Бинарник: `awg-portal_x86-64` (статический ELF, stripped)

### 1.2 Развёртывание и тестирование

**Стенды:**
- Сервер awg-portal: KVM VM (Ubuntu 24.04, Proxmox)
- Клиент (peer): KVM VM (Ubuntu 24.04, Proxmox)

**Порт 51820 — особенность:** на уровне гипервизора (Proxmox KVM) порт 51820 фильтруется для kernel WireGuard. AWG userspace на этом порту работает.

**Результаты тестов:**
| Тест | Результат |
|------|-----------|
| Kernel WireGuard (awg_mode: never, порт 51825) | ✅ Handshake, ping 5/5, RTT ~0.7ms |
| AmneziaWG userspace (awg_mode: auto, порт 51822) | ✅ TUN device, UAPI socket, процесс amneziawg-go |
| Локальный WG на сервере (127.0.0.1:51820) | ✅ Handshake, ping 0.03ms |
| UDP connectivity между нодами | ✅ Полный дуплекс |

### 1.3 Исправление: stale AWG socket (connection refused)

**Проблема:** при редактировании AWG-интерфейса через веб-интерфейс — `dial unix /var/run/amneziawg/wg0.sock: connect: connection refused`.

**Причина:** amneziawg-go падает (или перезапускается gateway), сокетный файл остаётся, процесс не слушает. `StartAWGProcess()` проверял только `os.Stat()` на наличие файла — мёртвый сокет считался живым.

**Исправление (2 файла):**

`internal/lowlevel/awg.go` — `StartAWGProcess()`:
- Вместо `os.Stat()` — `net.DialTimeout("unix", sock, 500ms)`
- Если сокет мёртв → `os.Remove(sock)` перед стартом

`internal/adapters/wgcontroller/local.go` — `getOrCreateInterface()`:
- При ошибке "connection refused" в AWG-режиме:
  1. `StopAWGProcess()` (pkill orphan)
  2. `os.Remove(sockPath)`
  3. `createLowLevelInterface()` (стартует свежий AWG-процесс)
  4. Retry `getInterface()`

`SocketPath()` экспортирована (была `socketPath()` — lowercase).

### 1.4 Выбор протокола (WG/AWG) в веб-интерфейсе

**Frontend:**
- `frontend/src/helpers/models.js` — `freshInterface()`: добавлены все AWG-поля (`AWGEnabled`, `AWGJc`–`AWGH4`)
- `frontend/src/components/InterfaceEditModal.vue`:
  - Toggle-переключатель AmneziaWG в карточке создания/редактирования интерфейса
  - Автоматическая генерация AWG-параметров при включении (совместимо с backend `GenerateAWGParams()`)
  - Поля для всех 12 параметров обфускации (Jc, Jmin, Jmax, S1-S4, H1-H4)
  - AWG-поля заполняются при редактировании существующего интерфейса

**API:**
- `POST /api/v0/interface/new` — уже поддерживает `AWGEnabled` и все AWG-поля
- `PUT /api/v0/interface/{id}` — уже поддерживает AWG-поля

### 1.5 Переименование в awg-portal

| Что было | Что стало |
|----------|-----------|
| `wg-portal-amd64` | `awg-portal_x86-64` |
| `/opt/wg-portal` | `/opt/awg-portal` |
| `wg-portal.service` | `awg-portal.service` |
| `/usr/local/bin/wg-portal` | `/usr/local/bin/awg-portal` |
| `WG_PORTAL_CONFIG=/opt/wg-portal/config.yml` | `WG_PORTAL_CONFIG=/opt/awg-portal/config.yml` |

---

## 2. Как развернуть

```bash
# Быстрый старт
wget https://github.com/DanilenkA/awg-portal/releases/download/v1.2.0/awg-portal-v1.2.0-bundle.tar.gz
tar xzf awg-portal-v1.2.0-bundle.tar.gz
cd awg-portal-v1.2.0/
sudo bash install.sh

# Редактировать конфиг
sudo nano /opt/awg-portal/config.yml

# Запустить
sudo systemctl enable --now awg-portal
```

**Конфиг для AWG-режима:**
```yaml
backend:
  awg_mode: auto    # auto | always | never
```

**Конфиг для kernel WG (если порт 51820 не заблокирован):**
```yaml
backend:
  awg_mode: never
```

---

## 3. Известные ограничения

1. **Порт 51820** — блокируется на уровне Proxmox/hypervisor на тестовых стендах. Для kernel WG использовать порт >51820.
2. **AWG-процессы** — после рестарта gateway воркеры `procs` map теряются. Реализовано восстановление при первом обращении (stale socket recovery), но при старте портала orphan-процессы не убиваются автоматически.
3. **Веб-интерфейс** — добавлен переключатель протокола только в EditModal, в таблице интерфейсов иконка/индикатор протокола не добавлены.
