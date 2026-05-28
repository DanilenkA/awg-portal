# v1.2.1fix — AWG Obfuscation Params Fix

**Дата:** 2026-05-28
**Скачать:** [awg-portal-v1.2.1fix-bundle.tar.gz](https://github.com/DanilenkA/awg-portal/releases/download/v1.2.1fix/awg-portal-v1.2.1fix-bundle.tar.gz)

## Исправления

### Критическое: AWG-параметры не передавались пирам через API

**Проблема:** При создании пира на AWG-интерфейсе с обфускацией, параметры
(Jc, Jmin, Jmax, S1–S4, H1–H4) генерировались в `PreparePeer()`, но терялись
при конвертации `domain.Peer → model.Peer` (и обратно) — AWG-полей просто
не было в API-моделях.

**Проявление:**
- В интерфейсе AWG включён, параметры видны
- Конфигурация клиента через скачивание/QR — содержит параметры
  (wg_peer.tpl читает domain.Peer напрямую)
- НО при создании пира через веб-интерфейс AWG-параметры в БД **не сохранялись**

**Корневая причина:** `NewPeer()` (domain → model) и `NewDomainPeer()` (model → domain)
не копировали AWG-поля. Фронтенд `freshPeer()` не имел AWG-полей для формы
создания/редактирования пира. Шаблоны `wg_peer.tpl` и `wg_interface.tpl`
не содержали `S3` и `S4` (были только S1, S2, H1-H4).

**Исправлено в файлах:**
| Файл | Изменение |
|------|-----------|
| `internal/app/api/v0/model/models_peer.go` | AWG поля в struct Peer, NewPeer(), NewDomainPeer() |
| `internal/app/api/v1/models/models_peer.go` | То же для v1 API |
| `frontend/src/helpers/models.js` | AWG поля в freshPeer() |
| `tpl_files/wg_peer.tpl` | Добавлены S3, S4 |
| `tpl_files/wg_interface.tpl` | Добавлены S3, S4 |

### Прямое управление AWG UAPI (v1.2.0+)

Добавлены методы `SetAWGPeer()`/`RemoveAWGPeer()` в `internal/lowlevel/awg.go`
для прямого управления пирами через UAPI-сокет amneziawg-go, в обход багов
парсинга wgctrl-go.

## Тестирование

Проверено на стенде:
- **Сервер:** Ubuntu 24.04, amneziawg-go, AWG-enabled интерфейс awg0 (10.130.130.100)
- **Клиент:** Ubuntu 24.04, vanilla WireGuard (kernel) + amneziawg-go (10.130.130.60)
- **AllowedIPs:** `10.211.1.0/24` и `10.211.2.0/24` (не `0.0.0.0/0`)

**Результаты:**
- ✅ **Vanilla WireGuard** (AWGEnabled=false): интерфейс wg0, пир, конфиг — OK, AWG-параметров нет
- ✅ **AmneziaWG** (AWGEnabled=true, Jc=4, Jmin=80, Jmax=180, S1–S4, H1–H4):
  PreparePeer возвращает **все** параметры (включая S3/S4, которых не было),
  пир сохраняется с AWG-параметрами в БД, конфиг клиента содержит все 11 параметров

## Установка

```bash
wget https://github.com/DanilenkA/awg-portal/releases/download/v1.2.1fix/awg-portal-v1.2.1fix-bundle.tar.gz
tar xzf awg-portal-v1.2.1fix-bundle.tar.gz
cd awg-portal-v1.2.1fix/
sudo bash install.sh
sudo nano /opt/awg-portal/config.yml   # настроить admin_user/admin_password
sudo systemctl enable --now awg-portal
```
