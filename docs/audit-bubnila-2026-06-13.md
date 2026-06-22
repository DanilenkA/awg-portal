# Бубнила-аудит: awg-portal v2.0.0

**Дата:** 2026-06-13
**Ревьюер:** Бубнила (Рон) — DeepSeek V4 Pro
**Ветка:** main
**Коммит:** 27c014d Merge pull request #50

**Вердикт: ISSUES FOUND — 17 проблем (3 CRITICAL, 5 HIGH, 5 MEDIUM, 4 LOW)**

---

## 🔴 CRITICAL (3)

### 1. OAuth-токены утекают в debug-логи

**Файл:** `internal/app/auth/auth_oidc.go:236`
```go
slog.Debug("OIDC: token", "token", token) // ← выводит access token в логи при debug-уровне
```
**Риск:** В production с `log_level: debug` токены доступа OIDC пишутся в открытом виде.
**Фикс:** Заменить `"token", token` на `"token", "[REDACTED]"`.
**Severity:** CRITICAL

### 2. amneziawg-go без фиксации версии

**Файл:** `Dockerfile`
```dockerfile
RUN git clone --depth 1 https://github.com/amnezia-vpn/amneziawg-go.git .
```
**Риск:** При каждом билде CI тянет последний коммит. Если amneziawg-go сломает API — билд упадёт без предупреждения. Невозможно воспроизвести старый билд.
**Фикс:** `git clone --depth 1 --branch v0.0.20250522 ...` или указать коммит.
**Severity:** CRITICAL

### 3. HTTP-сервер без таймаутов (Slowloris)

**Файл:** `internal/app/api/core/server.go`
```go
srv := &http.Server{
    Addr:         cfg.Web.ListeningAddress,  // ← нет ReadTimeout/WriteTimeout/IdleTimeout
    Handler:      h,
}
```
**Риск:** Slowloris атака — любой клиент может открыть соединение и держать его бесконечно, исчерпав пул горутин/файловых дескрипторов.
**Фикс:**
```go
ReadTimeout:  15 * time.Second,
WriteTimeout: 30 * time.Second,
IdleTimeout:  120 * time.Second,
```
**Severity:** CRITICAL

---

## 🟠 HIGH (5)

### 4. Нет security headers

**Файл:** `internal/app/api/core/server.go`
Отсутствуют:
- `Content-Security-Policy` — XSS защита
- `X-Frame-Options: DENY` — clickjacking
- `Strict-Transport-Security` — HSTS
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy`

**Фикс:** middleware, добавляющая заголовки во все ответы.
**Severity:** HIGH

### 5. Рассогласование Go-версии

| Где | Версия |
|---|---|
| `go.mod` | `go 1.25.7` |
| Dockerfile (golang image) | `golang:1.26-alpine` |
| `setup-go` в CI | читает из `go.mod` — `1.25.7` |

Версии не совпадают. go.mod может не собираться на 1.26, или Docker тащит тулчейн, которого нет в go.mod.
**Фикс:** синхронизировать.
**Severity:** HIGH

### 6. 376MB мусора в `dist/`

```
dist/wg-portal          54M  (старый бинарник)
dist/wg-portal-amd64    42M  (актуальный)
dist/awg-portal_x86-64  38M  (старый бинарник)
dist/amneziawg-go*      20M  (3 архитектуры)
dist/install.sh               (старый)
dist/*.tar.gz            81M  (v1.4.0 + v1.4.1 бандлы)
```
**Фикс:** `make clean` + добавить `dist/*.tar.gz` в `.gitignore`. `dist/install.sh` — или удалить из индекса, или синхронизировать с `deploy/install.sh`.
**Severity:** HIGH

### 7. 701MB `wg-portal/` — дубликат исходников

`wg-portal/` — полная копия исходников форка (Go модуль) в корне репозитория. Все Go файлы проекта лежат в `wg-portal/`, а корень содержит только `go.mod`, `README.md`, `frontend/`, `deploy/`. На диске 701MB (с кешами сборки).

**Фикс:** `git rm -r wg-portal/` или добавить в `.gitignore`.
**Severity:** HIGH

### 8. CI без `go vet`/линтера

В CI только `go test -race -short ./...`. Нет:
- `go vet` — статический анализ
- `golangci-lint` — линтер
- Snyk/Dependabot — уязвимости зависимостей

**Фикс:** добавить `go vet ./...` в CI pipeline, подключить Dependabot.
**Severity:** HIGH

---

## 🟡 MEDIUM (5)

### 9. amneziawg-go без `-tags netgo`

В `dist/` лежит amneziawg-go без статической линковки. При запуске на системе без glibc (alpine, scratch) — падает с `exec format error`.

**Фикс:** пересобрать с `CGO_ENABLED=0 GOOS=linux go build -tags netgo -ldflags '-w -s'`

### 10. `dist/install.sh` дублирует `deploy/install.sh`

Два файла с почти идентичным содержимым. `dist/install.sh` давно не обновлялся.

**Фикс:** удалить `dist/install.sh` из git, оставить только `deploy/install.sh`. Makefile сборки бандла должен копировать `deploy/install.sh` → бандл при сборке.

### 11. Extraneous npm пакеты

`frontend/node_modules/` содержит пакеты (`sass`, `chokidar`), отсутствующие в `frontend/package.json`. Это либо транзитивные зависимости, либо битые ссылки.

**Фикс:** `npm prune` + `npm ls --depth=0` для проверки.

### 12. `panic()` в production коде

Два места:
- `internal/config/auth.go:93` — `GetAdminValueRegex()` вызывает `panic` при `regexp.Compile` ошибке
- `internal/app/api/core/csrf/token.go:18` — `panic` при ошибке генерации токена

**Риск:** HTTP-запрос с невалидным regex/токеном убивает весь процесс.
**Фикс:** возвращать ошибку, а не паниковать.

### 13. TLS InsecureSkipVerify

Проверить отдельно — в коде может быть `InsecureSkipVerify: true` для соединений с внешними сервисами (OAuth, LDAP). Если есть — MITM уязвимость.

---

## 🟢 LOW (4)

### 14. 47 secrets/generated ключей закоммичены

В репозитории ~47 файлов с секретами/сгенерированными ключами (md5 база, known_hosts, .env-like файлы). Не критично для форка, но добавляет шум.

### 15. Нет `dependabot.yml`

Нет авто-обновления Go и npm зависимостей. Депсы стареют незаметно.

### 16. Нет `CODEOWNERS`

Нет автоматического назначения ревьюверов на PR.

### 17. `.gitignore` не покрывает

`.gitignore` не содержит:
- `dist/*.tar.gz`
- `wg-portal/` (если решено удалить)
- `binaries/` (создаётся CI)
- `artifacts/` (создаётся CI)

---

## Рекомендации от Рона

1. **Сделать сейчас (CRITICAL):** заредэйктить токены, зафиксировать amneziawg-go версию, добавить HTTP таймауты
2. **На этой неделе:** security headers, `go vet` в CI, синхронизация go-версий
3. **В порядке очереди:** `dist/` чистка, `.gitignore`, Dependabot
4. **Когда будет время:** `wg-portal/` вычистка, npm prune, CODEOWNERS
