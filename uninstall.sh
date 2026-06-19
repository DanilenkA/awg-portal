#!/usr/bin/env bash
#
# awg-portal uninstaller
#
# Полный purge awg-portal: останавливает и отключает сервис, удаляет бинарники
# /usr/local/bin/awg-portal и /usr/local/bin/amneziawg-go, снимает все
# WireGuard/AmneziaWG-интерфейсы через `ip link delete` и `wg-quick down`,
# удаляет /opt/awg-portal/ (данные и конфиги), пользователя awg-portal,
# runtime-каталог /run/amneziawg и systemd unit. Системные пакеты
# (wireguard-tools, iptables, openresolv) НЕ удаляются по умолчанию —
# для этого нужен явный флаг --purge-system-deps.
#
# Использование:
#     sudo ./uninstall.sh                 # интерактивное удаление
#     sudo ./uninstall.sh --yes           # без подтверждений (пакеты НЕ трогает)
#     sudo ./uninstall.sh --dry-run       # только показать, что будет удалено
#     sudo ./uninstall.sh --purge-system-deps   # + удалить системные пакеты
#     sudo ./uninstall.sh --keep-data     # оставить /opt/awg-portal/
#     sudo ./uninstall.sh --keep-user     # оставить пользователя awg-portal
#
# Идемпотентность: безопасно запускать повторно — отсутствующие файлы и
# пользователи не вызывают ошибок. Если уже всё удалено, скрипт просто
# сообщает «нечего удалять».
#
set -euo pipefail

VERSION="v2.0.2"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/awg-portal"
RUNTIME_DIR="/run/amneziawg"
WG_CONFIG_DIR="/etc/wireguard"
SERVICE_USER="awg-portal"
SERVICE_NAME="awg-portal"
SERVICE_UNIT="${SERVICE_NAME}.service"

# ─── Флаги ────────────────────────────────────────────────────────────────────
ASSUME_YES=0
DRY_RUN=0
PURGE_SYSTEM_DEPS=0
KEEP_DATA=0
KEEP_USER=0
for arg in "$@"; do
  case "$arg" in
    --yes|-y)               ASSUME_YES=1 ;;
    --dry-run|-n)           DRY_RUN=1 ;;
    --purge-system-deps)    PURGE_SYSTEM_DEPS=1 ;;
    --keep-data)            KEEP_DATA=1 ;;
    --keep-user)            KEEP_USER=1 ;;
    -h|--help)
      sed -n '2,/^set -euo pipefail/p' "$0" | sed '$d'
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Use --help for usage." >&2
      exit 2
      ;;
  esac
done

# ─── ANSI-коды ────────────────────────────────────────────────────────────────
YELLOW=$'\033[1;33m'
RESET=$'\033[0m'

# ─── Утилиты логирования ──────────────────────────────────────────────────────
log()  { printf '%b\n' "$*"; }
info() { log "  $*"; }
ok()   { log "  ✔ $*"; }
warn() { log "  ⚠ $*"; }
err() { log "  ✖ $*" >&2; }

if [ "$DRY_RUN" = "1" ]; then
  log "  ${YELLOW}[DRY-RUN]${RESET} ничего не удаляется, только показано, что произошло бы."
fi

run() {
  if [ "$DRY_RUN" = "1" ]; then
    info "[DRY-RUN] $*"
  else
    # shellcheck disable=SC2086  # намеренно: передаём $@ как список аргументов
    "$@"
  fi
}

ask_yes_no() {
  local prompt="$1" default="${2:-y}" reply
  if [ "$ASSUME_YES" = "1" ]; then return 0; fi
  if [ -n "${NONINTERACTIVE:-}" ]; then return 0; fi
  if [ "$default" = "y" ]; then
    prompt="$prompt [Y/n] "
  else
    prompt="$prompt [y/N] "
  fi
  read -r -p "$prompt" reply || return 1
  reply="${reply:-$default}"
  case "$reply" in
    y|Y|yes|YES) return 0 ;;
    *)           return 1 ;;
  esac
}

# ─── Root-проверка ────────────────────────────────────────────────────────────
if [ "$EUID" -ne 0 ]; then
  err "Скрипт нужно запустить от root: sudo ./uninstall.sh"
  exit 1
fi

# ─── План удаления (что и где) ────────────────────────────────────────────────
log ""
log "==> awg-portal uninstaller ${VERSION}"

if [ "$DRY_RUN" = "1" ]; then
  log "    режим: ${YELLOW}DRY-RUN${RESET} (ничего не удаляется)"
fi
log ""

# ─── 1. Сервис и unit ──────────────────────────────────────────────────────────
info "Шаг 1/6: Остановка и отключение systemd-сервиса"
if command -v systemctl >/dev/null 2>&1; then
  if systemctl list-unit-files "${SERVICE_UNIT}" >/dev/null 2>&1; then
    # is-active может вернуть unknown после stop, поэтому глушим ошибки.
    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
      run systemctl stop "${SERVICE_NAME}"
      ok "Сервис ${SERVICE_NAME} остановлен"
    else
      info "Сервис ${SERVICE_NAME} уже не запущен"
    fi
    if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
      run systemctl disable "${SERVICE_NAME}"
      ok "Сервис ${SERVICE_NAME} отключён из автозагрузки"
    else
      info "Сервис ${SERVICE_NAME} уже отключён"
    fi
  else
    info "Юнит ${SERVICE_UNIT} не зарегистрирован — пропускаем"
  fi
else
  warn "systemctl не найден — пропускаем управление сервисом"
fi

# ─── 2. Снятие WG/AWG интерфейсов ────────────────────────────────────────────
info "Шаг 2/6: Снятие WireGuard/AmneziaWG интерфейсов"
if command -v ip >/dev/null 2>&1; then
  # Список wireguard-подобных интерфейсов: wg*, awg*.
  interfaces=$(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep -E '^(wg|awg)[0-9a-zA-Z._-]*$' || true)
  if [ -z "$interfaces" ]; then
    info "Активных wg*/awg* интерфейсов не найдено"
  else
    for iface in $interfaces; do
      info "Найден интерфейс: ${iface}"
      if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Удалить ${iface}?" "y"; then
        # Сначала wg-quick down (если есть конфиг в /etc/wireguard) — он
        # аккуратно снимает маршруты и firewall-правила.
        if [ -f "${WG_CONFIG_DIR}/${iface}.conf" ] && command -v wg-quick >/dev/null 2>&1; then
          run wg-quick down "${iface}" || true
        fi
        run ip link delete dev "${iface}" || warn "  Не удалось удалить ${iface}"
        ok "Интерфейс ${iface} снят"
      fi
    done
  fi
else
  warn "ip(8) не найден — пропускаем снятие интерфейсов"
fi

# ─── 3. Бинарники ─────────────────────────────────────────────────────────────
info "Шаг 3/6: Удаление бинарников"
for bin in "${BIN_DIR}/awg-portal" "${BIN_DIR}/amneziawg-go"; do
  if [ -e "$bin" ] || [ -L "$bin" ]; then
    if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Удалить ${bin}?" "y"; then
      run rm -f "$bin"
      ok "Удалён ${bin}"
    fi
  else
    info "Не найден ${bin} — пропускаем"
  fi
done

# ─── 4. Данные и runtime-каталоги ─────────────────────────────────────────────
info "Шаг 4/6: Удаление /opt/awg-portal/ и /run/amneziawg"
if [ "$KEEP_DATA" = "1" ]; then
  warn "--keep-data: ${PORTAL_DIR} сохранён"
else
  if [ -d "$PORTAL_DIR" ]; then
    if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Удалить ${PORTAL_DIR} (данные и конфиги)?" "y"; then
      run rm -rf "$PORTAL_DIR"
      ok "Удалён ${PORTAL_DIR}"
    fi
  else
    info "Не найден ${PORTAL_DIR} — пропускаем"
  fi
fi
if [ -d "$RUNTIME_DIR" ]; then
  run rm -rf "$RUNTIME_DIR"
  ok "Удалён ${RUNTIME_DIR}"
fi
# Также удаляем каталоги .config и .cache от старых установок (необязательно).
for leftover in "/var/lib/${SERVICE_NAME}" "/var/log/${SERVICE_NAME}"; do
  if [ -d "$leftover" ]; then
    run rm -rf "$leftover"
    ok "Удалён ${leftover}"
  fi
done

# ─── 5. Пользователь и systemd unit ───────────────────────────────────────────
info "Шаг 5/6: Удаление пользователя и unit-файла"
if [ "$KEEP_USER" = "1" ]; then
  warn "--keep-user: пользователь ${SERVICE_USER} сохранён"
else
  if id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Удалить пользователя ${SERVICE_USER}?" "y"; then
      if run userdel "${SERVICE_USER}"; then
        ok "Удалён пользователь ${SERVICE_USER}"
      else
        warn "Не удалось удалить пользователя ${SERVICE_USER}"
        warn "  Возможно, им владеет работающий процесс. Убейте его и повторите:"
        warn "    pkill -u ${SERVICE_USER}; sudo userdel ${SERVICE_USER}"
      fi
    fi
  else
    info "Пользователь ${SERVICE_USER} не найден — пропускаем"
  fi
fi
# Unit-файл удаляем в конце (после остановки сервиса и его daemon-reload).
unit_path="${SYSTEMD_DIR}/${SERVICE_UNIT}"
if [ -f "$unit_path" ] || [ -L "$unit_path" ]; then
  run rm -f "$unit_path"
  ok "Удалён ${unit_path}"
fi
# Также удаляем «следы» blacklist, которые мог оставить install.sh (см. README).
if [ -f /etc/modprobe.d/blacklist-amneziawg.conf ]; then
  run rm -f /etc/modprobe.d/blacklist-amneziawg.conf
  ok "Удалён /etc/modprobe.d/blacklist-amneziawg.conf"
fi
if lsmod 2>/dev/null | grep -q '^amneziawg\b'; then
  if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Выгрузить модуль amneziawg из ядра?" "y"; then
    if run modprobe -r amneziawg; then
      ok "Модуль amneziawg выгружен"
    else
      warn "Не удалось выгрузить amneziawg (возможно, модуль занят)"
    fi
  fi
fi

# ─── 6. Системные пакеты (только по --purge-system-deps) ──────────────────────
info "Шаг 6/6: Системные пакеты"
if [ "$PURGE_SYSTEM_DEPS" != "1" ]; then
  info "Пропущено (нужен флаг --purge-system-deps)"
  info "  Если хотите удалить вручную:"
  info "    apt-get purge --auto-remove wireguard-tools iptables openresolv"
  info "    dnf remove wireguard-tools iptables openresolv"
  info "    pacman -Rns wireguard-tools iptables openresolv"
  info "    apk del wireguard-tools iptables openresolv"
else
  if [ "$DRY_RUN" = "1" ]; then
    info "[DRY-RUN] Удаление системных пакетов (по флагу --purge-system-deps)"
  fi
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    case "${ID:-}${ID_LIKE:-}" in
      *ubuntu*) PKG_MGR="apt-get"; FAMILY="debian" ;;
      *debian*) PKG_MGR="apt-get"; FAMILY="debian" ;;
      *fedora*) PKG_MGR="dnf";     FAMILY="rpm" ;;
      *rhel*|*centos*|*rocky*|*almalinux*) PKG_MGR="dnf"; FAMILY="rpm" ;;
      *arch*)    PKG_MGR="pacman"; FAMILY="arch" ;;
      *alpine*)  PKG_MGR="apk";    FAMILY="alpine" ;;
      *)         PKG_MGR="";        FAMILY="" ;;
    esac
  fi
  if [ -z "$PKG_MGR" ]; then
    warn "Не удалось определить пакетный менеджер — пропускаем"
  else
    case "$FAMILY" in
      debian) pkgs="wireguard-tools iptables openresolv" ;;
      rpm)    pkgs="wireguard-tools iptables openresolv" ;;
      arch)   pkgs="wireguard-tools iptables openresolv" ;;
      alpine) pkgs="wireguard-tools iptables openresolv" ;;
    esac
    info "Будут удалены пакеты: ${pkgs}"
    if [ "$ASSUME_YES" = "1" ] || ask_yes_no "  Продолжить?" "y"; then
      case "$PKG_MGR" in
        apt-get)
          export DEBIAN_FRONTEND=noninteractive
          # shellcheck disable=SC2086  # $pkgs намеренно без кавычек
          run apt-get purge -y --auto-remove $pkgs \
            || warn "apt-get purge завершился с ошибкой — проверьте вручную"
          ;;
        dnf)
          # shellcheck disable=SC2086
          run dnf remove -y $pkgs || warn "dnf remove завершился с ошибкой" ;;
        pacman)
          # shellcheck disable=SC2086
          run pacman -Rns --noconfirm $pkgs || warn "pacman завершился с ошибкой" ;;
        apk)
          # shellcheck disable=SC2086
          run apk del $pkgs || warn "apk del завершился с ошибкой" ;;
      esac
    fi
  fi
fi

# ─── Финал: daemon-reload ─────────────────────────────────────────────────────
if command -v systemctl >/dev/null 2>&1; then
  run systemctl daemon-reload
  run systemctl reset-failed "${SERVICE_NAME}" 2>/dev/null || true
fi

# ─── Итог ─────────────────────────────────────────────────────────────────────
log ""
log "==> Деинсталляция завершена."
log ""
if [ "$KEEP_DATA" = "1" ]; then
  log "Сохранено:"
  log "  • ${PORTAL_DIR}/ (--keep-data)"
fi
if [ "$KEEP_USER" = "1" ]; then
  log "Сохранено:"
  log "  • пользователь ${SERVICE_USER} (--keep-user)"
fi
log ""
log "Проверьте вручную, что:"
log "  • /etc/wireguard/ — если WG-конфиги больше не нужны, удалите отдельно"
log "  • правила iptables/nftables, добавленные awg-интерфейсами"
log "  • /var/log/journal* — старые логи сервиса"