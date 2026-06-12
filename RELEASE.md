# Release Process

## 1. Подготовка

```bash
cd /home/openclaw/projects/awg-portal

# Переключиться на main, подтянуть
git checkout main && git pull

# Убедиться что бинарники собраны
ls dist/wg-portal-* dist/amneziawg-go-*
```

## 2. Сборка бандла

```bash
VERSION="v<version>"
BUNDLE="/tmp/awg-portal-${VERSION}"
rm -rf "${BUNDLE}"
mkdir -p "${BUNDLE}/bin"

# wg-portal — все архитектуры
cp dist/wg-portal-amd64   "${BUNDLE}/bin/wg-portal-amd64"
cp dist/wg-portal-arm64   "${BUNDLE}/bin/wg-portal-arm64"
cp dist/wg-portal-arm     "${BUNDLE}/bin/wg-portal-arm"

# amneziawg-go — amd64 без суффикса, arm64/arm с суффиксом
cp dist/amneziawg-go-amd64  "${BUNDLE}/bin/amneziawg-go"
cp dist/amneziawg-go-arm64  "${BUNDLE}/bin/amneziawg-go-arm64"
cp dist/amneziawg-go-arm    "${BUNDLE}/bin/amneziawg-go-arm"

# install.sh (с версией) + config
cp deploy/install.sh      "${BUNDLE}/install.sh"
sed -i "s/VERSION=.*/VERSION=\"${VERSION}\"/" "${BUNDLE}/install.sh"
cp config.yml.sample      "${BUNDLE}/"

# Архив
cd /tmp && tar czf "awg-portal-${VERSION}-bundle.tar.gz" "awg-portal-${VERSION}"
```

## 3. Подпись бандла

```bash
cd /tmp

# SHA256 хеш
sha256sum awg-portal-<version>-bundle.tar.gz > SHA256SUMS

# GPG подпись ключом DanilenkA
gpg --default-key 95F966F1D92CB9D113F5E1E5C7EE105BD95C41BD \
  --detach-sign --armor \
  --output SHA256SUMS.sig \
  SHA256SUMS

# Проверка
cat SHA256SUMS
gpg --verify SHA256SUMS.sig SHA256SUMS
```

Файлы `SHA256SUMS` и `SHA256SUMS.sig` загружаются в релиз вместе с бандлом.

## 4. Проверка на стенде

```bash
# Очистить
sudo systemctl stop awg-portal 2>/dev/null
sudo systemctl disable awg-portal 2>/dev/null
sudo rm -f /usr/local/bin/awg-portal /usr/local/bin/amneziawg-go
sudo rm -rf /opt/awg-portal /etc/systemd/system/awg-portal.service /run/amneziawg
sudo systemctl daemon-reload

# Установить из бандла
sudo bash /tmp/awg-portal-v<version>/install.sh

# Запустить
sudo systemctl enable --now awg-portal
sleep 2
sudo systemctl status awg-portal --no-pager
journalctl -u awg-portal --no-pager -n 10

# Проверить что работает
curl -s -o /dev/null -w "%{http_code}" http://localhost:8888

# Очистить стенд
sudo systemctl stop awg-portal 2>/dev/null
sudo systemctl disable awg-portal 2>/dev/null
sudo rm -f /usr/local/bin/awg-portal /usr/local/bin/amneziawg-go
sudo rm -rf /opt/awg-portal /etc/systemd/system/awg-portal.service /run/amneziawg
sudo systemctl daemon-reload
```

## 5. Коммит и пуш

```bash
# Коммит изменений (install.sh, RELEASE.md и т.д.)
git add deploy/install.sh RELEASE.md
git commit -m "chore: update install.sh with arch detection, add RELEASE.md"
git push

# Создать подписанный тег
git tag -a -s "v<version>" -m "v<version> — <описание>"
git push origin "v<version>"
```

## 6. Публикация релиза

```bash
gh release create "v<version>" \
  --repo DanilenkA/awg-portal \
  --title "awg-portal v<version>" \
  --notes-file /tmp/release-notes.md \
  "/tmp/awg-portal-${VERSION}-bundle.tar.gz#awg-portal-${VERSION}-bundle.tar.gz" \
  "/tmp/SHA256SUMS#SHA256SUMS" \
  "/tmp/SHA256SUMS.sig#SHA256SUMS.sig"
```

Формат release-notes.md — как в прошлом релизе: изменения, состав бандла, инструкция (из бандла и через Docker).

## 7. Состав бандла

```
awg-portal-v<version>/
├── install.sh              ← скрипт установки (автовыбор архитектуры)
├── config.yml.sample       ← минимальный конфиг
└── bin/
    ├── wg-portal-amd64      + amneziawg-go          ← x86_64
    ├── wg-portal-arm64      + amneziawg-go-arm64    ← ARM64
    └── wg-portal-arm        + amneziawg-go-arm      ← ARM32
```
