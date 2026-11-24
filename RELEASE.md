# Release Process

Релиз собирается автоматически. Руками делаются только `VERSION` и `CHANGELOG.md`
в релизном PR. После мержа в `main` workflow `Release` создаёт тег, образ, бандл
и GitHub Release одним прогоном.

## 1. Подготовка релизного PR

1. `VERSION` содержит новый `vX.Y.Z`.
2. `make changelog` обновляет `CHANGELOG.md`.
3. `README.md` или `docs/` обновлены, если изменение видно пользователю.
4. `make verify-full` и `make release-check` зелёные.

## 2. Мерж

PR мержится в `main`. Ruleset требует PR, поэтому push в `main` означает мерж.

## 3. Что делает CI

1. Preflight: версия валидна, больше последнего тега, тег ещё не существует.
2. Verify: сборка фронтенда, тесты, линт, `changelog --check`, `release-check`.
3. Build: multi-arch образ `amd64/arm64/armv7`, теги `sha-<short>` и `main`.
4. Bundle: `VERSION`, `BUNDLE.md`, `MANIFEST.json`, `release-key.asc`, `install.sh`,
   `uninstall.sh`, `config.yml.sample`, шесть бинарников, `SHA256SUMS`.
5. Release: подписанный тег, ретиг `vX.Y.Z` и `latest`, `SHA256SUMS.sig`,
   provenance attestation, GitHub Release.
6. Chart: `helm package` с версией релиза и push в `oci://ghcr.io/<owner>/charts`.

## 4. Если релиз не нужен

CI при мерже без бампа `VERSION` публикует только образ `main`/`sha` и пропускает
релиз. Для PR, который меняет только документацию, workflow не запускается.

## 5. Ручной запуск

`workflow_dispatch` с `publish: false` собирает образ и бандл без публикации.
С `publish: true` выполняет полный релиз, но только на `main`/`master`.

## 6. Проверка релиза

```bash
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/awg-portal-bundle.tar.gz
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/SHA256SUMS
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/SHA256SUMS.sig
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/release-key.asc
gpg --import release-key.asc
gpg --verify SHA256SUMS.sig SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
gh attestation verify awg-portal-bundle.tar.gz --repo DanilenkA/awg-portal
```

Отпечаток ключа: `95F9 66F1 D92C B9D1 13F5 E1E5 C7EE 105B D95C 41BD`.

## 7. Ограничения

1. Подпись покрывает `SHA256SUMS`, а он покрывает внутренние файлы, версионированный
   архив и стабильный алиас.
2. Тег создаётся от `github-actions[bot]`; GitHub может показать его как Unverified,
   потому что ключ не привязан к боту. Криптографическая подпись при этом валидна.
3. Если ruleset запрещает push тега от `GITHUB_TOKEN`, шаг создания тега упадёт.
   В этом случае тег создаётся владельцем, а workflow запускается по тегу.
