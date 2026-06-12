# TESTING

**Актуально для:** v1.3.2+ (текущая ветка `handoff-memory-export`).
**Сценарий:** серверный smoke-тест портала через REST API + проверка деплоя.
**НЕ покрывает:** клиентский туннель (handshake/ping) — для него нужен отдельный
клиент-хост, см. полный протокол в [HANDOFF.md](HANDOFF.md) (gitignored, только
для регрессии на стенде).

---

## 0. Preflight — проверка стенда

Перед каждым тестом — полная очистка артефактов прошлых прогонов.

```bash
#!/bin/bash
# preflight.sh — должно быть идемпотентно (повторный запуск ничего не ломает)
set +e

# Сервис
sudo systemctl stop awg-portal 2>/dev/null
sudo systemctl disable awg-portal 2>/dev/null

# Интерфейсы (кроме lo/eth0/docker*)
for iface in $(ip -o link show | awk '{print $2}' | tr -d ':' | grep -E "^(wg|awg|tun)"); do
  sudo ip link delete "$iface" 2>/dev/null
done

# Процессы
sudo pkill -f amneziawg-go 2>/dev/null
sudo pkill -f wg-quick 2>/dev/null

# UAPI-сокеты
sudo rm -rf /run/amneziawg/ 2>/dev/null

# БД и конфиги
sudo rm -f /opt/awg-portal/data/sqlite.db 2>/dev/null
sudo rm -f /etc/wireguard/*.conf 2>/dev/null
sudo rm -f /tmp/wg-*.conf /tmp/awg-*.conf 2>/dev/null

echo "=== Интерфейсы (кроме lo/eth0/docker) ==="
ip -o link show | grep -vE "lo|eth0|docker" || echo "(чисто)"
echo "=== Процессы ==="
ps -eo pid,comm | grep -iE "amneziawg|wg-quick" | grep -v grep || echo "(чисто)"
echo "=== UAPI сокеты ==="
ls /run/amneziawg/ 2>/dev/null || echo "(чисто)"
echo "=== Порты 8888/8787 ==="
ss -tunelp 2>/dev/null | grep -E ":8888|:8787" || echo "(чисто)"
echo "=== Kernel-модули ==="
lsmod 2>/dev/null | grep -E "^(wireguard|amneziawg|tun) " || echo "(нет модулей — ок)"
```

**Правила стенда:**

| Правило | Зачем |
|---|---|
| Kernel-модуль `amneziawg` НЕ загружен | иначе amneziawg-go userspace откажется стартовать |
| Порты 8888/8787/51820+ свободны | иначе сервис не поднимется / клиент не подключится |
| Нет `awg-*`/`wg-*`/`tun*` интерфейсов | иначе тесты WG и AWG пересекутся с прошлым прогоном |
| Нет процессов `amneziawg-go` | могут оставить orphan UAPI-сокеты |
| `/run/amneziawg/` пуст или отсутствует | иначе amneziawg-go получит "address in use" |
| SQLite БД удалена | иначе тестовые пиры накопятся |

---

## 1. Установка

```bash
# 1. Собрать бинарник
make build-amd64
# Результат: dist/wg-portal-amd64 + dist/amneziawg-go

# 2. Установить через скрипт (тоже самое делает deploy/install.sh)
sudo bash dist/install.sh
# или
sudo bash deploy/install.sh
# Скрипт ищет бинарник в порядке:
#   dist/awg-portal_x86-64, dist/wg-portal, dist/wg-portal-amd64,
#   dist/wg-portal-arm64, dist/wg-portal-arm, dist/awg-portal,
#   awg-portal_x86-64, awg-portal (back compat)
# И устанавливает в /usr/local/bin/awg-portal независимо от исходного имени.

# 3. Подготовить конфиг
sudo cp /opt/awg-portal/config.yml.sample /opt/awg-portal/config.yml
sudo nano /opt/awg-portal/config.yml
# Обязательно:
#   core.admin_user: admin@awg.local
#   core.admin_password: <минимум 16 символов>

# 4. Запустить
sudo systemctl enable --now awg-portal
sudo systemctl status awg-portal --no-pager
ss -tunelp | grep 8888    # должен быть LISTEN
```

**Проверки после установки:**

- [ ] `/usr/local/bin/awg-portal --version` — не падает
- [ ] `systemctl is-active awg-portal` — `active`
- [ ] `ss -tunelp | grep 8888` — слушается
- [ ] `/etc/systemd/system/awg-portal.service` — содержит `ExecStart=/usr/local/bin/awg-portal`
- [ ] `/opt/awg-portal/config.yml` — существует, не дефолтный пароль

---

## 2. Smoke-тест WireGuard (через API)

Подсезть по умолчанию: `10.211.99.0/24`, порт `51899`. Допустимые `AllowedIPs`:
`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`. **Запрещено** `0.0.0.0/0`.

```bash
BASE="http://localhost:8888"
COOKIE=$(curl -s -c - -X POST "$BASE/api/v0/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@awg.local","password":"<ваш-пароль>"}' \
  | awk '/wgPortalSession/ {print $NF}')
test -n "$COOKIE" || { echo "FAIL: login failed"; exit 1; }

# 2.1. Создать WG-интерфейс
curl -s -X POST "$BASE/api/v0/interface/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{
    "Identifier":"wg-test",
    "Addresses":["10.211.99.1/24"],
    "ListenPort":51899,
    "PeerDefNetwork":["10.211.99.0/24"],
    "PeerDefAllowedIPs":["10.211.99.0/24"]
  }'
echo

# 2.2. Создать пира (prepare → new flow)
PEER=$(curl -s -X POST "$BASE/api/v0/peer/iface/wg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"Identifier":"wg-test-peer"}')
echo "Prepare response: $PEER"
curl -s -X POST "$BASE/api/v0/peer/iface/wg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d "$PEER"
echo

# 2.3. Скачать конфиг пира
PEER_ID=$(echo "$PEER" | python3 -c "import sys,json; print(json.load(sys.stdin)['Identifier'])")
curl -s "$BASE/api/v0/peer/config/$PEER_ID" \
  -H "Cookie: wgPortalSession=$COOKIE" -o /tmp/wg-test.json
# API возвращает JSON-строку, декодировать в .conf:
python3 -c "import json; print(json.load(open('/tmp/wg-test.json')))" > /tmp/wg-test.conf

# 2.4. Endpoint пустой — добавить руками (известная проблема API):
sed -i "s/^Endpoint = \$/Endpoint = <SERVER_IP>:51899/" /tmp/wg-test.conf

# 2.5. Проверки на сервере
ip link show wg-test          # интерфейс поднят
wg show wg-test publickey     # есть публичный ключ
ip route show dev wg-test     # connected route присутствует
ss -tunelp | grep 51899       # порт слушается
```

**Проверки (чек-лист):**

- [ ] Интерфейс `wg-test` существует и имеет адрес `10.211.99.1/24`
- [ ] `wg show wg-test` показывает `public key` (т.е. private key сгенерирован)
- [ ] `ip route show dev wg-test` содержит `10.211.99.0/24 dev wg-test scope link`
- [ ] `ss -tunelp | grep 51899` — `LISTEN` от kernel wireguard (`userspace="0"`)
- [ ] Пир в БД: `sqlite3 /opt/awg-portal/data/sqlite.db "SELECT identifier FROM peers WHERE interface_identifier='wg-test'"`
- [ ] Конфиг `/tmp/wg-test.conf` содержит `[Interface]` и `[Peer]` секции
- [ ] `awg-portal` НЕ запускал `amneziawg-go` для WG-интерфейса (`awg_mode: auto`)

**Известные ограничения:**

| Проблема | Workaround |
|---|---|
| `Endpoint = ` пустой в API-конфиге | `sed` после скачивания |
| API возвращает JSON-строку вместо raw .conf | `python3 -c "import json; print(json.load(...))"` |

---

## 3. Smoke-тест AmneziaWG (через API)

Подсезть по умолчанию: `10.212.99.0/24`, порт `51900`. Использует `amneziawg-go` userspace.

```bash
# 3.1. Создать AWG-интерфейс с обфускацией
curl -s -X POST "$BASE/api/v0/interface/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{
    "Identifier":"awg-test",
    "Addresses":["10.212.99.1/24"],
    "ListenPort":51900,
    "PeerDefNetwork":["10.212.99.0/24"],
    "PeerDefAllowedIPs":["10.212.99.0/24"],
    "AWGEnabled":true,
    "AWGJc":5,
    "AWGJmin":50,
    "AWGJmax":100,
    "AWGS1":10,
    "AWGS2":20,
    "AWGS3":30,
    "AWGS4":40,
    "AWGH1":100,
    "AWGH2":200,
    "AWGH3":300,
    "AWGH4":400
  }'
echo

# 3.2. Пир
PEER=$(curl -s -X POST "$BASE/api/v0/peer/iface/awg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"Identifier":"awg-test-peer"}')
curl -s -X POST "$BASE/api/v0/peer/iface/awg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d "$PEER"
echo

# 3.3. Конфиг
PEER_ID=$(echo "$PEER" | python3 -c "import sys,json; print(json.load(sys.stdin)['Identifier'])")
curl -s "$BASE/api/v0/peer/config/$PEER_ID" \
  -H "Cookie: wgPortalSession=$COOKIE" -o /tmp/awg-test.json
python3 -c "import json; print(json.load(open('/tmp/awg-test.json')))" > /tmp/awg-test.conf
sed -i "s/^Endpoint = \$/Endpoint = <SERVER_IP>:51900/" /tmp/awg-test.conf

# 3.4. Проверки
pgrep -af amneziawg-go         # процессы userspace
ip link show awg-test          # TUN-интерфейс поднят
ip route show dev awg-test     # connected route
ss -tunelp | grep 51900        # userspace listen (userspace="1")
ls /run/amneziawg/             # UAPI-сокеты
```

**Проверки (чек-лист):**

- [ ] Процесс `amneziawg-go` запущен (`pgrep -af amneziawg-go` непусто)
- [ ] Интерфейс `awg-test` существует (тип `tun`, не `wireguard`)
- [ ] `ip route show dev awg-test` содержит `10.212.99.0/24 dev awg-test scope link`
- [ ] `ss -tunelp | grep 51900` — `userspace:"amneziawg-go"` (НЕ kernel)
- [ ] `/run/amneziawg/awg-test.sock` существует
- [ ] В БД: `SELECT awg_enabled, awg_jc, awg_h1 FROM interfaces WHERE identifier='awg-test'`
  → `awg_enabled=1, awg_jc=5, awg_h1=100`
- [ ] В пире: `SELECT awg_jc, awg_s1, awg_h1 FROM peers WHERE interface_identifier='awg-test'`
  → те же значения, что у интерфейса
- [ ] Kernel-модуль `amneziawg` НЕ загружен (`lsmod | grep ^amneziawg` пусто)

**Известные ограничения:**

| Проблема | Workaround |
|---|---|
| `awg-quick` (kernel wg) не парсит `Jc=`/`S1=` | Использовать `awg-quick` из amnezia-tools |
| Endpoint пустой | `sed` после скачивания |

---

## 4. Параллельная работа WG + AWG

После шагов 2 и 3 оба интерфейса должны сосуществовать без конфликтов.

```bash
echo "=== WG ==="
ip -o link show | grep -E "wg-test"
ip route show dev wg-test
echo "=== AWG ==="
ip -o link show | grep -E "awg-test"
ip route show dev awg-test
echo "=== Процессы ==="
pgrep -af amneziawg-go
echo "=== Сокеты ==="
ls -la /run/amneziawg/
echo "=== Порты (разные) ==="
ss -tunelp | grep -E "51899|51900"
```

**Проверки:**

- [ ] `wg-test` — kernel WG, порт 51899
- [ ] `awg-test` — TUN + amneziawg-go, порт 51900
- [ ] Оба connected route присутствуют и НЕ пересекаются
- [ ] Маршруты к разным подсетям (`10.211.99.0/24` vs `10.212.99.0/24`)
- [ ] `/run/amneziawg/awg-test.sock` существует, НО НЕ `wg-test.sock`
- [ ] Только один процесс `amneziawg-go` (`pgrep -af amneziawg-go -c` == 1)

---

## 5. Docker compose тест

```bash
# Поднять стек
sudo docker compose up -d

# Проверки
sudo docker ps --filter name=awg-portal --format "{{.Names}}: {{.Status}}"
curl -fsS http://localhost:8888/api/v0/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@example.com","password":"<из env>"}'
ss -tunelp | grep -E ":8888|:8787"

# Логи
sudo docker logs --tail 50 awg-portal
```

**Проверки (чек-лист):**

- [ ] Контейнер `awg-portal` в статусе `Up` (не `Restarting`/`Exited`)
- [ ] Login API возвращает 200 (или 401 если креды неверные — но не 500)
- [ ] Порт 8888 слушается на хосте (`ss` покажет userspace "amneziawg" только для WG-порта)
- [ ] Порт 8787 (metrics) тоже слушается
- [ ] `docker exec awg-portal which amneziawg-go` — путь существует (внутри образа)
- [ ] `docker exec awg-portal ls -la /app/data` — БД создаётся
- [ ] Том `/etc/wireguard` доступен на запись
- [ ] `docker compose down` завершается без ошибок и БД сохраняется на хосте

**Команды очистки:**

```bash
sudo docker compose down
# БД остаётся в ./data, при необходимости:
sudo rm -rf ./data
```

---

## 6. Post-test cleanup

После завершения тестов стенд должен быть в исходном состоянии:

```bash
#!/bin/bash
# cleanup.sh
set +e

# Сервис
sudo systemctl stop awg-portal 2>/dev/null
sudo systemctl disable awg-portal 2>/dev/null

# Интерфейсы
for iface in $(ip -o link show | awk '{print $2}' | tr -d ':' | grep -E "^(wg|awg|tun)"); do
  sudo ip link delete "$iface" 2>/dev/null
done

# Процессы
sudo pkill -f amneziawg-go 2>/dev/null
sudo pkill -f wg-quick 2>/dev/null

# UAPI-сокеты
sudo rm -rf /run/amneziawg/ 2>/dev/null

# Файлы
sudo rm -f /etc/wireguard/*.conf 2>/dev/null
sudo rm -f /tmp/wg-*.conf /tmp/awg-*.conf 2>/dev/null

# БД
sudo rm -f /opt/awg-portal/data/sqlite.db 2>/dev/null

# Docker (если был)
sudo docker compose down 2>/dev/null
```

**После очистки — те же preflight-проверки, всё должно быть `(чисто)`.**

---

## Сводка сценариев

| # | Сценарий | Что проверяет | Стенд |
|---|---|---|---|
| 0 | Preflight | чистота стенда | сервер |
| 1 | Установка | install.sh + systemd | сервер |
| 2 | WG smoke | kernel WG + API peer flow | сервер |
| 3 | AWG smoke | userspace AWG + обфускация | сервер |
| 4 | WG+AWG параллельно | отсутствие конфликтов | сервер |
| 5 | Docker compose | контейнерный деплой | сервер |
| 6 | Cleanup | обратимость тестов | сервер |

Полный протокол с клиентским туннелем (ping через `wg-quick`/`awg-quick`,
handshake-проверками) — в [HANDOFF.md](HANDOFF.md) (gitignored, справочный).
