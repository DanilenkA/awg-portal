# v1.2.1fix — AWG Obfuscation Params Fix

## Исправления

### Критическое: AWG-параметры не передавались пирам через API

**Проблема:** При создании пира на AWG-интерфейсе с обфускацией, параметры
(Jc, Jmin, Jmax, S1–S4, H1–H4) генерировались в `PreparePeer()`, но терялись
при конвертации `domain.Peer → model.Peer` (и обратно) — AWG-полей просто
не было в API-моделях.

**Проявление:** В интерфейсе AWG включён, параметры видны. Конфигурация
клиента через скачивание/QR — содержит параметры (wg_peer.tpl читает
domain.Peer напрямую). Но при создании пира через веб-интерфейс AWG-параметры
в БД не сохранялись.

**Исправлено в файлах:**
- `internal/app/api/v0/model/models_peer.go` — AWG поля в struct Peer,
  NewPeer(), NewDomainPeer()
- `internal/app/api/v1/models/models_peer.go` — то же для v1 API
- `frontend/src/helpers/models.js` — AWG поля в freshPeer()
- `tpl_files/wg_peer.tpl` — добавлены S3, S4 (были пропущены)
- `tpl_files/wg_interface.tpl` — добавлены S3, S4

### Прямое управление AWG UAPI (v1.2.0+)

Добавлены методы `SetAWGPeer()`/`RemoveAWGPeer()` в `internal/lowlevel/awg.go`
для прямого управления пирами через UAPI-сокет amneziawg-go, в обход багов
парсинга wgctrl-go.

## Тестирование

Проверено на стенде:
- **Сервер:** Ubuntu 24.04, amneziawg-go, AWG-enabled интерфейс awg0
- **Клиент:** Ubuntu 24.04, vanila WireGuard (kernel)

**Результаты:**
- ✅ Vanilla WireGuard (AWGEnabled=false): интерфейс, пир, конфиг — OK
- ✅ AmneziaWG (AWGEnabled=true, Jc=4, Jmin=80, Jmax=180, S1–S4, H1–H4):
  PreparePeer возвращает параметры, пир сохраняется, конфиг содержит все 11
  параметров (включая S3 и S4, которых не было в шаблонах)
- ✅ AWG UAPI socket `/var/run/amneziawg/awg0.sock` активен
