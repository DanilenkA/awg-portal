# CLAUDE.md — AWG-Portal

Этот файл — контекст для Claude Code при работе с проектом awg-portal.

## О проекте
Форк h44z/wg-portal с поддержкой AmneziaWG (обфускация трафика).
WG использует kernel-модуль, AWG — userspace amneziawg-go через TUN-устройство.
Проект — VPN-панель с WebUI для управления интерфейсами и пирами WireGuard/AmneziaWG.

## Важные файлы
- `internal/lowlevel/awg.go` — управление процессом amneziawg-go (start/stop/watchdog)
- `internal/adapters/wgcontroller/local.go` — физический слой: create/update/delete интерфейсов
- `internal/app/wireguard/wireguard_interfaces.go` — Manager слой: saveInterface, createInterface
- `internal/domain/interface.go` — модели AWGParams, Interface, валидация
- `internal/app/configfile/tpl_files/wg_peer.tpl` — шаблон конфига пира
- `frontend/src/components/InterfaceEditModal.vue` — WebUI модалка интерфейсов
- `.github/workflows/docker-publish.yml` — CI/CD

## Ключевые константы
- AWG modes: `auto` (default), `always` (force), `never` (kernel only)
- AWG params ranges: Jc [0-128], Jmin/Jmax [0-1280], S1-S4 [0-1280], H1-H4 [5, 2^32-1]
- UAPI socket: `/var/run/amneziawg/<iface>.sock`
- Start timeout: 10s (awgProcessStartTimeout)
- Dial timeout: 500ms (awgUAPIDialTimeout)

## Архитектурные решения
- `Interface.EmitAWG()` — единый метод для "отправлять ли AWG params?"
- `awgModeChanged()` — детектит только toggle AWGEnabled, не изменение params
- `awgProcEntry{cmd, done}` — entry в procs, watchdog закрывает done после Wait()
- Watchdog — единственный владелец cmd.Wait()
- Валидация AWG params — в `Interface.Validate()`, на серверной стороне
- DB save — ПОСЛЕ физических операций (но не атомарно)

## Что нельзя делать
- Не менять `wgctrl-go` — Jipok-форк с AWG-поддержкой
- Не ломать kernel WG при добавлении AWG-фич
- Не использовать goreleaser — только GitHub Actions
- Не пушить в main без PR
- Не хардкодить версии — через ldflags

## История фиксов (10.06.2026)

### Round 1 — Claude Opus 4.8 (аудит + 5 приоритетов)
Mode switch WG↔AWG, awg_mode=never, lifecycle amneziawg-go, server-side validation, no silent downgrade

### Round 2 — Claude Opus 4.8 (Codex review #1)
awg_mode=always для всех, порядок teardown-before-save, EmitAWG(), peer restore disabled, watchdog

### Round 3 — Claude Opus 4.8 (Codex review #2)
DB save переупорядочен, HasAnyAWGParams исправлен, SIGINT→SIGTERM, awgProcEntry

### Round 4 — Codex GPT-5.5 (Codex review #3 + финальная добивка)
- **Watchdog data race** — ProcessState заменён на `entry.stopping` флаг
- **DB-before-physical атомарность** — rollback при ошибке
- **Double-start window** — StopAWGProcess не удаляет entry до выхода процесса
- **Комментарии** — приведены в соответствие с кодом
- **Stale socket** — string match → typed unwrap
- **CI** — добавлен `go test` job в docker-publish.yml
- **Frontend** — Math.random → crypto.getRandomValues
- **IsZero** — проверяет все поля AWG (было только Jc+H1-H4)

### Известные pre-existing (не наши)
1. Тест TestSqlRepo_SaveInterface_Simple — flaky (shared in-memory SQLite)
2. CI: semver-теги дают 403 (type=semver в metadata-action)

## Процесс работы
1. `go build ./...` после изменений
2. `go vet ./...` после изменений
3. `go test -race -count=3 ./internal/lowlevel/ ./internal/domain/ ./internal/app/wireguard/`
4. `go test ./...` — все 15 пакетов должны быть зелёные
5. Коммитить только после зелёных тестов
6. Не удалять pre-existing тесты (могут быть flaky)

---

## Пост-деплойное тестирование (только после сборки релиза)

Полный протокол: HANDOFF.md раздел "Протокол тестирования"

### 0. Жёсткая pre-test проверка
Перед любым тестом обе ноды проверяются и очищаются.
Не должно быть:
- wg*/awg*/tun* интерфейсов
- процессов awg/amnezia/wg-quick
- UAPI сокетов (/run/amneziawg/)
- старой БД /opt/awg-portal/data/sqlite.db
- старых конфигов /etc/wireguard/*.conf
- старых маршрутов туннелей
- портов 8888/8787

### Правило: два интерфейса на сервере, под каждый протокол свой
```
Сервер 10.130.130.100:
  wg-test  (WireGuard)    → 10.211.99.1/24  port 51899  (kernel)
  awg-test (AmneziaWG)    → 10.212.99.1/24  port 51900  (userspace)

Клиент 10.130.130.60:
  wg-test  → 10.211.99.2/32
  awg-test → 10.212.99.2/32
```
Оба интерфейса создаются независимо, работают одновременно.
Адресные пространства не пересекаются.

**AllowedIPs:** только 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16. Запрещён 0.0.0.0/0. У пира — только /32.

### Сценарии (по порядку)
1. **WG smoke** — создать wg-test → пира → поднять на клиенте → ping 10.211.99.1
2. **AWG smoke** — создать awg-test → пира → поднять на клиенте → ping 10.212.99.1
3. **Параллельная WG+AWG** — оба интерфейса подняты одновременно → пинг через оба
4. **Регрессия WG** — WG работает, несмотря на AWG на соседнем интерфейсе

### Известные проблемы
- connected route убивается setRoutesForFamily()
- nil pointer в getOrCreateRoutingTableAndFwMark (local.go:1045)
