# AWG-Portal — Handoff

Извлечено из MEMORY.md 05.06.2026

## Проект awg-portal (AWG-PORTAL)
- Репозиторий: https://github.com/DanilenkA/awg-portal
- Default branch: `main`, PR обязателен
- Remote: `danilenka` (SSH git@github.com:DanilenkA/awg-portal.git)
- Локальный путь: /home/openclaw/projects/awg-portal/wg-portal
- go.mod: `module github.com/DanilenkA/awg-portal`, go 1.25.7
- Version ldflag: `-X 'github.com/DanilenkA/awg-portal/internal.Version=...'`
- **Контекст проекта:** `read memory/awg-portal-context.md` перед работой

### Релизный бандл
- awg-portal_x86-64 (встроенный фронтенд, ~42 MB)
- amneziawg-go (~3.3 MB)
- install.sh + config.yml.sample

### AWG-параметры обфускации
Параметры (Jc, Jmin, Jmax, S1-S4, H1-H4) рапространяются через 6 точек:
- PreparePeer(), saveInterface(), savePeers(), ApplyInterfaceDefaults(), importPeer()
- savePeers() — force-propagate (фикс v1.2.2fix)
- Шаблон wg_peer.tpl: `{{- if .Peer.Interface.AWGEnabled}}`

### Тестовый стенд
- Сервер: 10.130.130.100:8888 (SSH locadmin@10.130.130.100, ключ ~/.ssh/id_ed25519)
- Клиент: 10.130.130.60
- Сервис: awg-portal.service (systemd)
- Конфиг: /opt/awg-portal/config.yml
- БД: /opt/awg-portal/data/sqlite.db
- Админ: admin@awg.local

## Процедура тестирования awg-portal

### 0. Проверка окружения (перед любым тестом)
  - **а0)** Модули ядра: `lsmod | grep -E "wireguard|amnezia|tun"` — amneziawg быть не должно
  - **б0)** Порты: `ss -tunelp` — 8888/8787 заняты только если тестовый сервис запущен
  - **в0)** Процессы: `ps aux | grep -iE "awg|portal|amnezia"` — не должно быть мусора
  - **г0)** Интерфейсы: `ip a | grep -E "^[0-9]+"` — только lo, eth0, docker0. Лишние (wg, awg, tun) — удалить.
  - **д0)** Docker: `docker ps -a` — стопнуть и удалить контейнеры от прошлых тестов (`docker rm -f`)
  - **е0)** Blacklist модуля: проверить что `/etc/modprobe.d/blacklist-amneziawg.conf` есть (если не нужен)

### 1. Запуск и тест
- **а)** Стартовать сервис со стендовым конфигом, проверить что стартанул (journalctl, порты)
- **б)** Создать WG-интерфейс и пира, поднять туннель на клиенте
- **в)** Пинг с клиента на сервер через туннель:
  Сервер 10.130.130.100 → wg/awg адрес 10.211.x.1/24
  Клиент 10.130.130.60 → wg/awg адрес 10.211.x.2/24
  Поднять соединение на клиенте, пинговать 10.211.x.1.
- **г)** AllowedIPs — только /24 подсеть wg/awg интерфейса. Никогда 0.0.0.0/0.
- **д)** Если тест проходит на WG, но не на AWG — дебажить и чинить так, чтобы не сломать WG, и наоборот.

### 2. Пост-тест (cleanup)
- **а)** Остановить сервис, удалить systemd unit/transient
- **б)** Удалить конфиги со стендов (`rm -rf /etc/wireguard/*.conf /home/locadmin/.../*.conf`)
- **в)** Удалить БД (`rm -f .../data/sqlite.db`)
- **г)** Удалить лишние интерфейсы (wg, awg, tun)
- **д)** Убить процессы amneziawg-go
- **е)** Удалить UAPI сокеты (`rm -f /run/amneziawg/*.sock`)
- **ж)** Проверить чистоту по пункту 0 (чтобы следующий тест стартовал с чистого листа)

### 3. Релизные правила
- **а)** Не пушить, пока есть явные баги или недоработки.
- **б)** Репозиторий — только `git@github.com:DanilenkA/awg-portal.git`.
- **в)** После каждого релиза проверять и актуализировать README.
- **г)** В релиз класть только tar.gz бандл с install.sh, не сырые бинарники
- **д)** Название ассета: `awg-portal-v{VERSION}-bundle.tar.gz` (не `wg-portal_linux_*`)
- **е)** Только x86-64 (arm/arm64 без тестов не выкладывать)
- **ж)** .goreleaser не использовать — всё через GitHub Actions + Docker

### 4. CI грабли (31.05.2026)
- Export amneziawg-go — только на тегах (`if: startsWith(github.ref, 'refs/tags/v')`), иначе PR ломается
- `${{ env.* }}` в GitHub Actions вычисляется на compile-time, нельзя читать в том же `run:` блоке где пишется `>> $GITHUB_ENV`
- Версию в install.sh проставлять через sed при сборке, не хардкодить
- `ubuntu-latest` — плавающий тег, фиксировать `ubuntu-24.04`

### 5. connected route — частый баг awg-portal
- `setRoutesForFamily()` удаляет kernel-connected route `X.X.X.0/24 dev Y scope link` из main-таблицы
- HACK восстановления — для ВСЕХ интерфейсов, не только AWG
- Симптом: handshake есть, data transfer есть, пинги 100% loss
- Диагностика: `ip route show dev <iface>` — если пусто, маршрут убит
- Фикс: `internal/adapters/wgcontroller/local.go` — connected route re-add вне AWG-условия

