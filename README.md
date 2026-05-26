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

## Запуск

TODO
