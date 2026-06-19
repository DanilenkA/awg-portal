# TESTING

**Актуально для:** v2.0.0 (ветка `feature/ui-ux-v2.0.0`).
**Стенды:**
- Сервер: 10.130.130.100 (порт 8888)
- Клиент: 10.130.130.60
- Креды: admin@example.com / secretsecter123456789

---

## 0. Preflight — проверка стендов

Перед каждым тестом — полная очистка артефактов прошлых прогонов **на обоих стендах**.

### Сервер (10.130.130.100)

```bash
ssh openclaw@10.130.130.100
sudo systemctl stop awg-portal 2>/dev/null; sudo systemctl disable awg-portal 2>/dev/null
for iface in $(ip -o link show | awk '{print $2}' | tr -d ':' | grep -E "^(wg|awg|tun)"); do sudo ip link delete "$iface" 2>/dev/null; done
sudo pkill -f amneziawg-go 2>/dev/null; sudo pkill -f wg-quick 2>/dev/null; sudo pkill -f awg-portal 2>/dev/null
sudo rm -rf /run/amneziawg/ /opt/awg-portal/ 2>/dev/null
sudo rm -f /etc/wireguard/*.conf /usr/local/bin/awg-portal /usr/local/bin/amneziawg-go /etc/systemd/system/awg-portal.service 2>/dev/null
sudo systemctl daemon-reload 2>/dev/null
echo "=== ПРОВЕРКА ==="
ip -o link show | grep -vE "lo|eth0|docker" || echo "(чисто)"
ss -tunelp | grep -E ":8888|:8787|:51899|:51900" || echo "(чисто)"
lsmod 2>/dev/null | grep -E "^(amneziawg) " || echo "(нет amneziawg module)"
```

### Клиент (10.130.130.60)

```bash
ssh openclaw@10.130.130.60
sudo systemctl stop awg-portal 2>/dev/null; sudo systemctl disable awg-portal 2>/dev/null
for iface in $(ip -o link show | awk '{print $2}' | tr -d ':' | grep -E "^(wg|awg|tun)"); do sudo ip link delete "$iface" 2>/dev/null; done
sudo pkill -f amneziawg-go 2>/dev/null; sudo pkill -f wg-quick 2>/dev/null
sudo rm -rf /run/amneziawg/ /opt/awg-portal/ 2>/dev/null
sudo rm -f /etc/wireguard/*.conf /usr/local/bin/awg-portal /usr/local/bin/amneziawg-go 2>/dev/null
echo "=== ПРОВЕРКА ==="
ip -o link show | grep -vE "lo|eth0|docker" || echo "(чисто)"
```

---

## 0.5. Проверка бандла (до установки)

### 0.5.1. Структура

```
awg-portal-v2.0.0/
├── install.sh           ✔ корень
├── uninstall.sh         ✔ корень
├── config.yml.sample    ✔ корень
└── bin/
    ├── wg-portal-amd64  ✔
    ├── wg-portal-arm64  ✔
    ├── wg-portal-arm    ✔
    └── amneziawg-go     ✔
```

**Проверки:**
- [ ] `install.sh` в корне
- [ ] `uninstall.sh` в корне
- [ ] `config.yml.sample` в корне
- [ ] `bin/` существует
- [ ] `bin/wg-portal-amd64` существует
- [ ] `bin/amneziawg-go` существует

### 0.5.2. Архитектура бинарников

```bash
file bin/wg-portal-amd64
file bin/wg-portal-arm64
file bin/wg-portal-arm
file bin/amneziawg-go
```

**Проверки:**
- [ ] `wg-portal-amd64` — `ELF 64-bit LSB executable, x86-64, statically linked, stripped`
- [ ] `wg-portal-arm64` — `ELF 64-bit LSB executable, ARM aarch64, statically linked, stripped`
- [ ] `wg-portal-arm` — `ELF 32-bit LSB executable, ARM, EABI5 version 1, statically linked, stripped`
- [ ] `amneziawg-go` — `ELF 64-bit LSB executable, x86-64, statically linked, stripped`
- [ ] Нигде нет `dynamically linked`

### 0.5.3. Размеры

- [ ] `wg-portal-amd64` — не более 45MB (~41MB)
- [ ] `wg-portal-arm64` — не более 45MB (~39MB)
- [ ] `wg-portal-arm` — не более 45MB (~39MB)
- [ ] `amneziawg-go` — не более 5MB (~3.3MB)
- [ ] Бандл — не более 130MB
- [ ] Архив `.tar.gz` — не более 50MB

### 0.5.4. Соответствие install.sh ожиданиям

- [ ] `bin/wg-portal-amd64` находится find_binary() для amd64
- [ ] `bin/wg-portal-arm64` находится для arm64
- [ ] `bin/wg-portal-arm` находится для arm
- [ ] `bin/amneziawg-go` находится

### 0.5.5. `install.sh --help`

- [ ] exit code 0
- [ ] Выводит описание

### 0.5.6. Распаковка архива

```bash
rm -rf /tmp/bundle-test
tar xzf awg-portal-v2.0.0.tar.gz -C /tmp/bundle-test --strip-components=1
ls -la /tmp/bundle-test/
```

- [ ] `install.sh` executable
- [ ] `uninstall.sh` executable
- [ ] `config.yml.sample` присутствует
- [ ] `bin/` с бинарниками

---

## 1. Установка на сервер (install.sh)

```bash
# Копировать бандл на сервер
scp /tmp/awg-portal-v2.0.0.tar.gz openclaw@10.130.130.100:/tmp/
ssh openclaw@10.130.130.100

# Распаковать
rm -rf /tmp/awg-portal-bundle
mkdir -p /tmp/awg-portal-bundle
tar xzf /tmp/awg-portal-v2.0.0.tar.gz -C /tmp/awg-portal-bundle --strip-components=1

# 1.1. Интерактивная установка
sudo bash /tmp/awg-portal-bundle/install.sh
```

**Проверки (1.1):**
- [ ] `/usr/local/bin/awg-portal` — существует, executable, statically linked
- [ ] `/usr/local/bin/amneziawg-go` — существует, executable
- [ ] `/opt/awg-portal/config.yml` — существует, IP подставлен (не `<IP>`)
- [ ] `/opt/awg-portal/data/` и `/opt/awg-portal/config/` — существуют
- [ ] `/run/amneziawg/` — существует, владелец `awg-portal`
- [ ] Пользователь `awg-portal` создан (`id awg-portal`)
- [ ] `/etc/systemd/system/awg-portal.service` — содержит `ExecStart=/usr/local/bin/awg-portal`
- [ ] `systemctl daemon-reload` проходит

### 1.2. `--auto-install-deps`

```bash
# Удалить wireguard-tools для чистоты теста
sudo apt-get remove -y wireguard-tools openresolv 2>/dev/null || true

sudo bash /tmp/awg-portal-bundle/install.sh --auto-install-deps
```

- [ ] `wg` доступен (`command -v wg`)
- [ ] `iptables` доступен
- [ ] Модуль `wireguard` загружен (`lsmod | grep ^wireguard`)
- [ ] Установка без вопросов (NONINTERACTIVE)
- [ ] Все пункты из 1.1

### 1.3. Идемпотентность

```bash
sudo bash /tmp/awg-portal-bundle/install.sh
```

- [ ] exit code 0
- [ ] Сообщения "уже существует"
- [ ] `/opt/awg-portal/config.yml` НЕ перезаписан (mtime не изменился)

### 1.4. `--no-install-deps`

```bash
sudo bash /tmp/awg-portal-bundle/install.sh --no-install-deps
```

- [ ] Бинарники + unit установлены
- [ ] Системные пакеты НЕ устанавливались

---

## 2. Smoke-тест WireGuard (через API)

```bash
# Запустить сервис
sudo systemctl enable --now awg-portal
sleep 3
systemctl status awg-portal --no-pager
```

```bash
BASE="http://localhost:8888"
COOKIE=$(curl -s -c - -X POST "$BASE/api/v0/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@example.com","password":"secretsecter123456789"}' \
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

# 2.2. Создать пира
PEER=$(curl -s -X POST "$BASE/api/v0/peer/iface/wg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"Identifier":"wg-client"}')
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
python3 -c "import json; print(json.load(open('/tmp/wg-test.json')))" > /tmp/wg-test.conf
sed -i "s/^Endpoint = \$/Endpoint = 10.130.130.100:51899/" /tmp/wg-test.conf

# 2.4. Проверки на сервере
ip link show wg-test
wg show wg-test publickey
ip route show dev wg-test
ss -tunelp | grep 51899
```

**Проверки:**
- [ ] `systemctl is-active awg-portal` — active
- [ ] `ss -tunelp | grep 8888` — слушается
- [ ] Login возвращает cookie (не 401/500)
- [ ] `wg-test` существует, адрес `10.211.99.1/24`
- [ ] `wg show wg-test` показывает public key
- [ ] `ip route show dev wg-test` содержит `10.211.99.0/24 dev wg-test scope link`
- [ ] `ss -tunelp | grep 51899` — `LISTEN` от kernel wireguard
- [ ] Пир в БД: `sudo sqlite3 /opt/awg-portal/data/sqlite.db "SELECT identifier FROM peers WHERE interface_identifier='wg-test'"`
- [ ] Конфиг `/tmp/wg-test.conf` содержит `[Interface]` и `[Peer]` секции
- [ ] `amneziawg-go` НЕ запущен для WG-интерфейса

---

## 3. Smoke-тест AmneziaWG (через API)

```bash
# 3.1. AWG-интерфейс с обфускацией
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

# 3.2. Пир для AWG
PEER_AWG=$(curl -s -X POST "$BASE/api/v0/peer/iface/awg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d '{"Identifier":"awg-client"}')
echo "Prepare response: $PEER_AWG"
curl -s -X POST "$BASE/api/v0/peer/iface/awg-test/new" \
  -H "Cookie: wgPortalSession=$COOKIE" \
  -H "Content-Type: application/json" \
  -d "$PEER_AWG"
echo

# 3.3. Конфиг AWG-пира
PEER_AWG_ID=$(echo "$PEER_AWG" | python3 -c "import sys,json; print(json.load(sys.stdin)['Identifier'])")
curl -s "$BASE/api/v0/peer/config/$PEER_AWG_ID" \
  -H "Cookie: wgPortalSession=$COOKIE" -o /tmp/awg-test.json
python3 -c "import json; print(json.load(open('/tmp/awg-test.json')))" > /tmp/awg-test.conf
sed -i "s/^Endpoint = \$/Endpoint = 10.130.130.100:51900/" /tmp/awg-test.conf

# 3.4. Проверки
pgrep -af amneziawg-go
ip link show awg-test
ip route show dev awg-test
ss -tunelp | grep 51900
ls /run/amneziawg/
```

**Проверки:**
- [ ] Процесс `amneziawg-go` запущен
- [ ] `systemctl is-active awg-portal` — active (AWG crash не убил сервис)
- [ ] `awg-test` существует (тип `tun`)
- [ ] `ip route show dev awg-test` содержит `10.212.99.0/24 dev awg-test scope link`
- [ ] `ss -tunelp | grep 51900` — userspace ("amneziawg-go"), НЕ kernel
- [ ] `/run/amneziawg/awg-test.sock` существует
- [ ] В БД: `sudo sqlite3 /opt/awg-portal/data/sqlite.db "SELECT awg_enabled FROM interfaces WHERE identifier='awg-test'"` → `1`
- [ ] Kernel-модуль `amneziawg` НЕ загружен

---

## 4. Параллельная работа WG + AWG

```bash
echo "=== WG ==="
ip -o link show | grep -E "wg-test"
ip route show dev wg-test
echo "=== AWG ==="
ip -o link show | grep -E "awg-test"
ip route show dev awg-test
echo "=== Сокеты ==="
ls -la /run/amneziawg/
echo "=== Порты ==="
ss -tunelp | grep -E "51899|51900"
```

**Проверки:**
- [ ] `wg-test` — kernel WG, порт 51899
- [ ] `awg-test` — TUN + amneziawg-go, порт 51900
- [ ] Маршруты не пересекаются (`10.211.99.0/24` vs `10.212.99.0/24`)
- [ ] `/run/amneziawg/awg-test.sock` существует, `wg-test.sock` НЕ существует
- [ ] Только один процесс `amneziawg-go`

---

## 5. Клиентский туннель (WG + AWG с клиента)

### 5.1. Скопировать конфиги пиров на клиент

```bash
# На сервере: скопировать конфиги на клиент
scp /tmp/wg-test.conf openclaw@10.130.130.60:/tmp/
scp /tmp/awg-test.conf openclaw@10.130.130.60:/tmp/
```

### 5.2. Поднять WG-туннель на клиенте и пинговать

```bash
ssh openclaw@10.130.130.60

# Поднять WG-интерфейс через wg-quick
sudo wg-quick up /tmp/wg-test.conf
echo "=== WG status ==="
sudo wg show
echo "=== IP ==="
ip addr show wg-test
echo "=== Ping WG server ==="
ping -c 4 -W 2 10.211.99.1
```

**Проверки (5.2):**
- [ ] `wg-test` на клиенте поднят
- [ ] `wg show` показывает handshake с сервером
- [ ] `ping -c 4 10.211.99.1` — 0% loss
- [ ] RTT < 100ms

### 5.3. Поднять AWG-туннель на клиенте и пинговать

```bash
# Для AWG нужен amneziawg-go на клиенте
# Скопировать бинарник
# scp openclaw@10.130.130.100:/usr/local/bin/amneziawg-go /tmp/amneziawg-go

# AWG использует amneziawg-go для поднятия интерфейса
# Проверить что конфиг содержит Jc/Jmin/Jmax и т.д.
grep -E "Jc|S1|H1" /tmp/awg-test.conf

# Поднять AWG через wg-quick (если установлен amnezia-tools)
# Или вручную через amneziawg-go
sudo wg-quick up /tmp/awg-test.conf 2>&1 || \
  echo "Требуется amnezia-wg-quick для AWG"

echo "=== Ping AWG server ==="
ping -c 4 -W 2 10.212.99.1
```

**Проверки (5.3):**
- [ ] AWG интерфейс поднят на клиенте
- [ ] Конфиг содержит AWG-параметры (Jc, S1, H1 и т.д.)
- [ ] `ping -c 4 10.212.99.1` — 0% loss

### 5.4. Оба туннеля одновременно

```bash
echo "=== WG show ==="
sudo wg show
echo "=== Routes ==="
ip route show | grep -E "10\.211\.99|10\.212\.99"
echo "=== Pings ==="
ping -c 2 -W 1 10.211.99.1
ping -c 2 -W 1 10.212.99.1
```

**Проверки (5.4):**
- [ ] Оба интерфейса активны одновременно
- [ ] Оба ping проходят
- [ ] Маршруты не конфликтуют

---

## 6. Счётчики трафика (cumulative)

После клиентских пингов — проверить, что счётчики на сервере отображаются корректно.

```bash
# На сервере
PEER_ID_B64=$(printf 'wg-test' | base64 -w0)
curl -s -b "wgPortalSession=$COOKIE" \
  "http://localhost:8888/api/v0/peer/iface/${PEER_ID_B64}/stats" | python3 -m json.tool
```

**Проверки:**
- [ ] `BytesReceived` > 0 (после пингов)
- [ ] `BytesTransmitted` > 0
- [ ] `BytesReceived` / `BytesTransmitted` — integer (не string)
- [ ] В бандле интерфейса отображается Totals (cumulative), не 0 B/s

---

## 7. Деинсталляция (uninstall.sh)

### 7.1. Dry-run

```bash
sudo bash /tmp/awg-portal-bundle/uninstall.sh --dry-run
```

- [ ] Вывод содержит `[DRY-RUN]` перед каждым действием
- [ ] Ничего не удалено на самом деле (`/opt/awg-portal`, `/usr/local/bin/awg-portal` существуют)
- [ ] systemd unit не остановлен

### 7.2. Полный purge

```bash
sudo bash /tmp/awg-portal-bundle/uninstall.sh --yes
```

- [ ] Сервис остановлен и отключён
- [ ] WG/AWG интерфейсы сняты
- [ ] `/usr/local/bin/awg-portal` — удалён
- [ ] `/usr/local/bin/amneziawg-go` — удалён
- [ ] `/opt/awg-portal/` — удалён
- [ ] `/run/amneziawg/` — удалён
- [ ] Пользователь `awg-portal` — удалён
- [ ] `/etc/systemd/system/awg-portal.service` — удалён
- [ ] Системные пакеты НЕ удалены (не был передан `--purge-system-deps`)

### 7.3. purge с сохранением данных

```bash
# Установить заново
sudo bash /tmp/awg-portal-bundle/install.sh --auto-install-deps

# Удалить с --keep-data
sudo bash /tmp/awg-portal-bundle/uninstall.sh --yes --keep-data
```

- [ ] `/opt/awg-portal/` — сохранён
- [ ] Всё остальное удалено (бинарники, пользователь, unit)

### 7.4. Идемпотентность

```bash
# После успешного purge
sudo bash /tmp/awg-portal-bundle/uninstall.sh --yes
```

- [ ] exit code 0
- [ ] Все действия — "пропускаем" (ничего не удаляется повторно)

---

## 8. Cleanup

Вернуть оба стенда в исходное состояние (выполнить preflight на сервере и клиенте).

---

## Сводка сценариев

| # | Сценарий | Стенд | Что проверяет |
|---|---|---|---|
| 0 | Preflight | оба | чистота стендов |
| 0.5 | Бандл | сборка | структура, архитектура, размеры |
| 1.1 | Установка интерактивная | сервер | install.sh |
| 1.2 | Установка --auto-install-deps | сервер | зависимости без вопросов |
| 1.3 | Идемпотентность install.sh | сервер | повторный запуск |
| 1.4 | Установка --no-install-deps | сервер | только бинарники |
| 2 | WG smoke | сервер | kernel WG + API |
| 3 | AWG smoke | сервер | userspace AWG + обфускация |
| 4 | WG+AWG параллельно | сервер | отсутствие конфликтов |
| 5.2 | Клиент WG туннель | клиент→сервер | ping 10.211.99.1 |
| 5.3 | Клиент AWG туннель | клиент→сервер | ping 10.212.99.1 |
| 5.4 | Оба туннеля | клиент→сервер | параллельная работа |
| 6 | Счётчики | сервер | cumulative Totals |
| 7.1 | Dry-run | сервер | uninstall.sh --dry-run |
| 7.2 | Полный purge | сервер | uninstall.sh --yes |
| 7.3 | --keep-data | сервер | сохранение конфига |
| 7.4 | Идемпотентность uninstall | сервер | повторный запуск |
| 8 | Cleanup | оба | возврат в исходное |
