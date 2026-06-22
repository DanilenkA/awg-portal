#!/usr/bin/env bash
#
# awg-portal installation script
#
# Идемпотентный установщик для чистой машины:
#   - определяет дистрибутив (debian/ubuntu/fedora/arch/alpine/rhel) и
#     архитектуру (amd64/arm64/arm);
#   - ставит системные зависимости (wireguard-tools, модуль ядра wireguard,
#     resolvconf, iptables) — после интерактивного подтверждения или при
#     наличии флага --auto-install-deps;
#   - копирует бинарь awg-portal и amneziawg-go из бандла в /usr/local/bin/;
#   - создаёт системного пользователя awg-portal, директории /opt/awg-portal
#     и /run/amneziawg;
#   - копирует config.yml.sample (один раз), генерирует systemd-юнит;
#   - безопасно работает при повторном запуске (не переустанавливает то, что
#     уже есть, не затирает существующий config.yml).
#
# Использование:
#     sudo ./install.sh                  # интерактивный режим
#     sudo ./install.sh --auto-install-deps   # без вопросов про apt-get
#     sudo ./install.sh --no-install-deps     # только положить бинарь
#                                             #   и systemd unit
#
# Поддерживаемые бандлы:
#     awg-portal-vX.Y.Z/
#       install.sh
#       config.yml.sample
#       bin/wg-portal-amd64
#       bin/wg-portal-arm64
#       bin/wg-portal-arm
#       bin/amneziawg-go
#
set -euo pipefail

# ─── Версия и пути ────────────────────────────────────────────────────────────
VERSION="v2.0.2"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/awg-portal"
RUNTIME_DIR="/run/amneziawg"
SERVICE_USER="awg-portal"
SERVICE_NAME="awg-portal"

# ─── Флаги ────────────────────────────────────────────────────────────────────
AUTO_INSTALL_DEPS=0
NO_INSTALL_DEPS=0
for arg in "$@"; do
  case "$arg" in
    --auto-install-deps) AUTO_INSTALL_DEPS=1 ;;
    --no-install-deps)   NO_INSTALL_DEPS=1   ;;
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
if [ "$AUTO_INSTALL_DEPS" = "1" ] && [ "$NO_INSTALL_DEPS" = "1" ]; then
  echo "ERROR: --auto-install-deps and --no-install-deps are mutually exclusive." >&2
  exit 2
fi

# ─── Layout бандла ────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# BUNDLE_DIR — корень бандла. install.sh может лежать:
#   - в корне бандла (SCRIPT_DIR == BUNDLE_DIR) — целевой layout;
#   - в dist/ после `make build` (тогда поднимаемся на уровень);
#   - в deploy/ (legacy layout, оставлено для обратной совместимости).
if [ -d "${SCRIPT_DIR}/bin" ] || \
   [ -f "${SCRIPT_DIR}/config.yml.sample" ] || \
   [ -f "${SCRIPT_DIR}/wg-portal" ]; then
  BUNDLE_DIR="${SCRIPT_DIR}"
elif [ -d "${SCRIPT_DIR}/../bin" ]; then
  BUNDLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  BUNDLE_DIR="${SCRIPT_DIR}"
fi

# ─── Утилиты логирования ──────────────────────────────────────────────────────
log()  { printf '%b\n' "$*"; }
info() { log "  $*"; }
ok()   { log "  ✔ $*"; }
warn() { log "  ⚠ $*"; }
err()  { log "  ✖ $*" >&2; }
step() { log ""; log "[$1/$TOTAL_STEPS] $2"; }
ask_yes_no() {
  local prompt="$1" default="${2:-y}" reply
  if [ "$AUTO_INSTALL_DEPS" = "1" ]; then return 0; fi
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
  err "Скрипт нужно запустить от root: sudo ./install.sh"
  exit 1
fi

# ─── Определение архитектуры ───────────────────────────────────────────────────
ARCH=""
case "$(uname -m)" in
  x86_64|amd64)        ARCH="amd64"  ;;
  aarch64|arm64)       ARCH="arm64"  ;;
  armv7l|armv7|armv8l) ARCH="arm" ;;
  # riscv64)           ARCH="riscv64" ;;   # см. CHECKLIST Бубнилы — не собирается CI
  *)
    err "Неизвестная архитектура: $(uname -m)"
    err "Поддерживаются: amd64, arm64, arm."
    exit 1
    ;;
esac

# ─── Определение дистрибутива ─────────────────────────────────────────────────
DISTRO=""        # debian | ubuntu | fedora | arch | alpine | rhel
DISTRO_FAMILY="" # debian | rpm | arch | alpine
PKG_MGR=""       # apt | dnf | yum | pacman | apk

if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  case "${ID:-}${ID_LIKE:-}" in
    *ubuntu*) DISTRO="ubuntu"; DISTRO_FAMILY="debian"; PKG_MGR="apt-get" ;;
    *debian*) DISTRO="debian"; DISTRO_FAMILY="debian"; PKG_MGR="apt-get" ;;
    *fedora*) DISTRO="fedora"; DISTRO_FAMILY="rpm";    PKG_MGR="dnf"     ;;
    *rhel*|*centos*|*rocky*|*almalinux*)
                DISTRO="${ID:-rhel}"; DISTRO_FAMILY="rpm"; PKG_MGR="dnf"  ;;
    *arch*)    DISTRO="arch";   DISTRO_FAMILY="arch";  PKG_MGR="pacman"   ;;
    *alpine*)  DISTRO="alpine"; DISTRO_FAMILY="alpine"; PKG_MGR="apk"     ;;
    *)
      warn "Неизвестный дистрибутив: ID=${ID:-?}, ID_LIKE=${ID_LIKE:-?}"
      warn "Системные пакеты ставиться не будут. Продолжаем."
      ;;
  esac
else
  warn "/etc/os-release не читается — пропускаем установку системных пакетов."
fi

# ─── Маппинг пакетов по дистрибутиву ─────────────────────────────────────────
# Возвращает список пакетов, разделённых пробелами.
pkg_list() {
  case "$DISTRO_FAMILY" in
    debian)
      local pkgs="wireguard-tools iptables kmod"
      # openresolv: на Ubuntu 22+ не нужен (systemd-resolved), и пакет
      # отсутствует в репозитории Ubuntu 24.04. На более старых Ubuntu и
      # на Debian он всё ещё актуален для wg-quick.
      if [ "$DISTRO" != "ubuntu" ] || [ -z "${VERSION_ID:-}" ] || [ "${VERSION_ID%%.*}" -lt 22 ] 2>/dev/null; then
        pkgs="$pkgs openresolv"
      fi
      echo "$pkgs linux-headers-$(uname -r)"
      ;;
    rpm)
      echo "wireguard-tools iptables openresolv kmod"
      ;;
    arch)
      # kernel-headers нужен только если собирать модуль из исходников,
      # но чаще достаточно стандартного linux.
      echo "wireguard-tools iptables openresolv"
      ;;
    alpine)
      echo "wireguard-tools iptables openrespv kmod"
      ;;
    *)
      echo ""
      ;;
  esac
}

# ─── Установка системных пакетов ──────────────────────────────────────────────
install_system_deps() {
  if [ "$NO_INSTALL_DEPS" = "1" ]; then
    info "Режим --no-install-deps: системные пакеты пропущены."
    return 0
  fi
  if [ -z "$PKG_MGR" ]; then
    warn "Не удалось определить пакетный менеджер — пропускаем установку зависимостей."
    return 0
  fi

  local pkgs
  pkgs="$(pkg_list)"
  if [ -z "$pkgs" ]; then
    warn "Список пакетов для $DISTRO пуст — пропускаем."
    return 0
  fi

  # Проверка: если все пакеты уже установлены — не переустанавливаем.
  local missing=()
  local p
  for p in $pkgs; do
    case "$PKG_MGR" in
      apt-get)
        if ! dpkg -s "$p" >/dev/null 2>&1; then missing+=("$p"); fi
        ;;
      dnf|yum)
        if ! rpm -q "$p" >/dev/null 2>&1; then missing+=("$p"); fi
        ;;
      pacman)
        if ! pacman -Q "$p" >/dev/null 2>&1; then missing+=("$p"); fi
        ;;
      apk)
        if ! apk info -e "$p" >/dev/null 2>&1; then missing+=("$p"); fi
        ;;
    esac
  done

  if [ "${#missing[@]}" -eq 0 ]; then
    ok "Все системные зависимости уже установлены: $pkgs"
    return 0
  fi

  log "  Требуется установить: ${missing[*]}"
  if ! ask_yes_no "Установить сейчас через ${PKG_MGR}?" "y"; then
    warn "Установка пакетов пропущена по запросу пользователя."
    return 0
  fi

  case "$PKG_MGR" in
    apt-get)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update
      apt-get install -y --no-install-recommends "${missing[@]}"
      ;;
    dnf)
      dnf install -y "${missing[@]}"
      ;;
    yum)
      yum install -y "${missing[@]}"
      ;;
    pacman)
      pacman -Sy --noconfirm "${missing[@]}"
      ;;
    apk)
      apk add --no-cache "${missing[@]}"
      ;;
  esac
  ok "Системные зависимости установлены."
}

# ─── Проверки TUN/wireguard ───────────────────────────────────────────────────
check_runtime_prereqs() {
  log ""
  log "==> Проверка рантайм-предпосылок"

  # 1. /dev/net/tun
  if [ -e /dev/net/tun ]; then
    ok "/dev/net/tun существует"
  else
    err "/dev/net/tun отсутствует."
    err "  AWG (userspace) требует TUN. Включите модуль ядра или запустите контейнер с /dev/net/tun."
    err "  Быстрый фикс для OpenVZ/LXC:    mkdir -p /dev/net && mknod /dev/net/tun c 10 200 && chmod 600 /dev/net/tun"
    err "  Для VM с обычным ядром: обычно всё ок, проверьте lsmod | grep tun"
  fi

  # 2. wireguard kernel module (опционально: awg может работать без него,
  #    но для обычного WG и ip link add type wireguard он нужен).
  if modinfo wireguard >/dev/null 2>&1; then
    ok "Модуль ядра wireguard доступен"
    if ! lsmod | grep -q '^wireguard\b'; then
      if [ "$AUTO_INSTALL_DEPS" = "1" ] || [ -n "${NONINTERACTIVE:-}" ]; then
        if modprobe wireguard 2>/dev/null; then
          ok "Модуль wireguard загружен"
        else
          warn "Не удалось загрузить wireguard (modprobe)."
        fi
      else
        if ask_yes_no "Загрузить модуль wireguard сейчас?" "y"; then
          if modprobe wireguard 2>/dev/null; then
            ok "Модуль wireguard загружен"
          else
            warn "Не удалось загрузить wireguard."
          fi
        fi
      fi
    fi
  else
    warn "Модуль wireguard не найден в ядре."
    warn "  Обычный WG работать не сможет, только AWG userspace через amneziawg-go."
  fi

  # 3. wg / ip
  if command -v wg >/dev/null 2>&1; then
    ok "wg(8) доступен: $(command -v wg)"
  else
    warn "wg(8) не найден. Порт сможет управлять интерфейсами только через netlink."
  fi
  if command -v ip >/dev/null 2>&1; then
    ok "ip(8) доступен"
  else
    err "ip(8) не найден — это критично. Установите iproute2."
  fi
  if command -v systemctl >/dev/null 2>&1; then
    ok "systemctl доступен"
  else
    warn "systemctl не найден — systemd unit не будет установлен."
  fi
}

# ─── Поиск бинарника ──────────────────────────────────────────────────────────
find_binary() {
  local generic="${1}"
  local arch_only="${2:-}"
  local candidate
  # Сначала строгий поиск по архитектуре: bin/<x>-<arch>, <x>-<arch>, dist/<x>-<arch>.
  if [ -n "$arch_only" ]; then
    for candidate in \
        "${BUNDLE_DIR}/bin/${arch_only}-${ARCH}" \
        "${BUNDLE_DIR}/${arch_only}-${ARCH}" \
        "${BUNDLE_DIR}/dist/${arch_only}-${ARCH}"; do
      if [ -f "$candidate" ]; then
        printf '%s\n' "$candidate"
        return 0
      fi
    done
  fi
  # Fallback: плоские имена — в т.ч. в dist/ (где `make build` их кладёт).
  for candidate in \
      "${BUNDLE_DIR}/bin/${generic}" \
      "${BUNDLE_DIR}/${generic}" \
      "${BUNDLE_DIR}/dist/${generic}" \
      "${BUNDLE_DIR}/dist/${generic}-${ARCH}"; do
    if [ -f "$candidate" ]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

install_awg_portal_binary() {
  # 1. wg-portal (Go-бинарь портала).
  local src dst
  if src="$(find_binary "wg-portal" "wg-portal")"; then
    :
  else
    err "Бинарник awg-portal не найден для ARCH=${ARCH}."
    err "  Ожидается один из: ${BUNDLE_DIR}/bin/wg-portal-{amd64,arm64,arm}"
    err "  или плоский: ${BUNDLE_DIR}/wg-portal"
    err "  Пересоберите бандл: make build-amd64 (или все цели)."
    exit 1
  fi
  dst="${BIN_DIR}/${SERVICE_NAME}"
  install -m 0755 "$src" "$dst"
  ok "Установлен ${dst} (из ${src##*/})"
}

install_amneziawg_binary() {
  local src dst
  if src="$(find_binary "amneziawg-go" "amneziawg-go")"; then
    dst="${BIN_DIR}/amneziawg-go"
    install -m 0755 "$src" "$dst"
    ok "Установлен ${dst} (из ${src##*/})"
  else
    warn "amneziawg-go не найден в бандле — AWG (обфускация) работать не будет,"
    warn "  обычный WG (kernel) останется доступен."
    warn "  Соберите: cd amneziawg-go && CGO_ENABLED=0 go build -tags netgo ."
  fi
}

# ─── Системный пользователь и каталоги ───────────────────────────────────────
ensure_user() {
  if id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    ok "Пользователь ${SERVICE_USER} уже существует"
  else
    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "Пользователь ${SERVICE_USER} создан"
  fi
}

ensure_dirs() {
  mkdir -p "${PORTAL_DIR}/data" "${PORTAL_DIR}/config"
  mkdir -p "${RUNTIME_DIR}"
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "${RUNTIME_DIR}"
  chmod 0750 "${RUNTIME_DIR}"
}

# ─── Конфиг ──────────────────────────────────────────────────────────────────
install_config() {
  local src candidate primary_ip
  for candidate in \
      "${BUNDLE_DIR}/config.yml.sample" \
      "${BUNDLE_DIR}/config.yml" \
      "${SCRIPT_DIR}/config.yml.sample"; do
    if [ -f "$candidate" ]; then
      src="$candidate"
      break
    fi
  done

  if [ -z "${src:-}" ]; then
    warn "config.yml.sample не найден — пропускаем."
    return 0
  fi

  if [ -f "${PORTAL_DIR}/config.yml" ]; then
    ok "Конфиг уже существует: ${PORTAL_DIR}/config.yml — оставляем как есть."
    return 0
  fi

  primary_ip="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '/src/ {for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}')"
  primary_ip="${primary_ip:-<IP>}"

  cp "$src" "${PORTAL_DIR}/config.yml"
  # Подставляем реальный IP вместо плейсхолдера.
  sed -i "s|<IP>|${primary_ip}|g" "${PORTAL_DIR}/config.yml"
  sed -i "s|http://<IP>|http://${primary_ip}|g" "${PORTAL_DIR}/config.yml"
  chown "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}/config.yml"
  ok "Конфиг создан: ${PORTAL_DIR}/config.yml (IP=${primary_ip})"
  warn "** Обязательно отредактируйте ${PORTAL_DIR}/config.yml — смените admin_password перед запуском! **"
}

chown_portal_dir() {
  chown -R "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}"
}

# ─── Systemd unit ─────────────────────────────────────────────────────────────
install_systemd_unit() {
  if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemctl недоступен — пропускаем установку unit-файла."
    return 0
  fi

  local unit_path="${SYSTEMD_DIR}/${SERVICE_NAME}.service"
  local unit_content
  unit_content=$(cat <<'UNIT'
[Unit]
Description=AWG Portal — Web UI for WireGuard & AmneziaWG
Documentation=https://github.com/DanilenkA/awg-portal
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=awg-portal
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
Restart=on-failure
RestartSec=3
StartLimitBurst=5

WorkingDirectory=/opt/awg-portal
Environment=WG_PORTAL_CONFIG=/opt/awg-portal/config.yml
ExecStart=/usr/local/bin/awg-portal
RuntimeDirectory=amneziawg
RuntimeDirectoryMode=0750

# Hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/opt/awg-portal /etc/wireguard /run/amneziawg
ReadOnlyPaths=-/dev/net/tun
RestrictAddressFamilies=AF_NETLINK AF_INET AF_INET6 AF_UNIX
RestrictRealtime=true
RestrictSUIDSGID=true
MemoryDenyWriteExecute=true
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW

[Install]
WantedBy=multi-user.target
UNIT
)

  # Идемпотентная переустановка: переписываем unit, если он отличается.
  if [ -f "$unit_path" ] && [ "$(cat "$unit_path" 2>/dev/null || true)" = "$unit_content" ]; then
    ok "Systemd unit уже актуален: $unit_path"
  else
    printf '%s\n' "$unit_content" > "$unit_path"
    chmod 0644 "$unit_path"
    ok "Systemd unit установлен: $unit_path"
  fi

  systemctl daemon-reload
}

# ─── Главный поток ────────────────────────────────────────────────────────────
TOTAL_STEPS=6
log ""
log "==> awg-portal installer ${VERSION}"
log "    arch:     ${ARCH}"
if [ -n "$DISTRO" ]; then
  log "    distro:   ${DISTRO} (${DISTRO_FAMILY}, ${PKG_MGR})"
else
  log "    distro:   unknown"
fi
log "    bundle:   ${BUNDLE_DIR}"

step 1 "Проверка рантайм-предпосылок"
check_runtime_prereqs

step 2 "Установка системных зависимостей"
install_system_deps

step 3 "Установка бинарников"
install_awg_portal_binary
install_amneziawg_binary

step 4 "Создание пользователя и каталогов"
ensure_user
ensure_dirs

step 5 "Развёртывание конфигурации"
install_config
chown_portal_dir

step 6 "Установка systemd unit"
install_systemd_unit

# ─── Итог ─────────────────────────────────────────────────────────────────────
log ""
log "==> Установка завершена."
log ""
log "Установлено:"
log "  • ${BIN_DIR}/${SERVICE_NAME}"
if [ -x "${BIN_DIR}/amneziawg-go" ]; then
  log "  • ${BIN_DIR}/amneziawg-go"
fi
log "  • ${PORTAL_DIR}/{data,config}/"
log "  • ${PORTAL_DIR}/config.yml"
log "  • ${SYSTEMD_DIR}/${SERVICE_NAME}.service"
log ""
log "Дальнейшие шаги:"
log "  1. ${WARN}Измените ${PORTAL_DIR}/config.yml (admin_user/admin_password).${RESET}"
log "  2. Включите и запустите сервис:"
log "         sudo systemctl enable --now ${SERVICE_NAME}"
log "  3. Проверьте статус:"
log "         sudo systemctl status ${SERVICE_NAME}"
log "         sudo journalctl -u ${SERVICE_NAME} -f"
log ""
log "Подсказки по AmneziaWG:"
log "  • Модуль ядра amneziawg НЕ требуется — amneziawg-go работает в userspace."
log "  • Требуется /dev/net/tun (уже проверено)."
log "  • awg_mode: auto в config.yml выберет AWG только для интерфейсов с обфускацией."
log "  • Если в ядре есть модуль amneziawg и он конфликтует — занесите в blacklist:"
log "         echo 'blacklist amneziawg' | sudo tee /etc/modprobe.d/blacklist-amneziawg.conf"

# ANSI-коды оставляем как переменные на случай подстановки выше.
WARN=$'\033[1;33m'
RESET=$'\033[0m'