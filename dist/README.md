# awg-portal

> Web-портал управления AmneziaWG через адаптированный wireguard-go.

## Архитектура

```
wg-portal UI/API → wireguard.go (adapter)
 │
 ├─ wgctrl.ConfigureDevice() ← БЕЗ изменений
 └─ lowlevel.ApplyAWGParams() ← НОВЫЙ вызов
 │
 /var/run/wireguard/wg0.sock
 │
 amneziawg-go (systemd)
```

### Компоненты

- **[wg-portal](https://github.com/h44z/wg-portal)** — UI/API для управления WireGuard-сервером
- **[wireguard-go](https://github.com/WireGuard/wireguard-go)** — userspace-адаптер, через который wg-portal управляет интерфейсами
- **[amneziawg-go](https://github.com/amneia-vpn/amneziawg-go)** — изменённый wg-core с поддержкой AmneziaWG-протокола

### Сращивание

- `wgctrl.ConfigureDevice()` — используется как есть, для стандартных WG-параметров
- `lowlevel.ApplyAWGParams()` — новый вызов, прокидывающий AmneziaWG-специфичные параметры (Jc, Jmin, Jmax, S1, S2, H1, H2, H3, H4) поверх стандартного конфига
- Взаимодействие с amneziawg-go через Unix-сокет `/var/run/wireguard/wg0.sock`

## Релизная сборка

```bash
make dist
```

Артефакты — в `dist/`:

```
dist/
├── wg-portal-amd64          # статический бинарник (stripped, CGO_ENABLED=0)
├── wg-portal.service        # systemd-юнит портала
├── amneziawg@.service       # systemd-юнит userspace-демона AmneziaWG
├── override-wg-portal.conf  # override: зависимость wg-portal от amneziawg@wg0
├── install.sh               # скрипт установки на целевую систему
├── README.md
└── VERSION
```

### Пересборка при обновлении upstream

```bash
git submodule update --remote  # обновить wg-portal, amneziawg-go, wireguard-go
go version
make dist
```

## Развёртывание

```bash
# На целевой системе (Linux + systemd, от root):
sudo bash dist/install.sh

# Или вручную:
sudo install -m 0755 dist/wg-portal-amd64 /usr/local/bin/wg-portal
sudo systemctl daemon-reload
sudo systemctl enable --now amneziawg@wg0
sudo systemctl start wg-portal
```

См. [оригинальную документацию wg-portal](https://github.com/h44z/wg-portal) по настройке конфига.

