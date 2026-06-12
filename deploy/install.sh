#!/bin/bash
# awg-portal installation script
#
# Compatible with multiple bundle layouts:
#   v1.4.0 release bundle (install.sh at root, binaries in bin/):
#       awg-portal-v1.4.0/
#         install.sh
#         config.yml.sample
#         bin/wg-portal-amd64, bin/amneziawg-go, ...
#   Old/legacy bundle (binaries at root, install.sh in deploy/):
#       awg-portal_x86-64, amneziawg-go, config.yml.sample
#       deploy/install.sh
#   Local build (after `make build-amd64`):
#       dist/wg-portal-amd64, dist/amneziawg-go, dist/install.sh
#
# Run as root: sudo bash install.sh   (or sudo bash deploy/install.sh)
set -euo pipefail

VERSION="v1.4.0"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/awg-portal"
SERVICE_USER="awg-portal"

# Определение архитектуры хоста
ARCH=""
case "$(uname -m)" in
  x86_64|amd64)        ARCH="amd64" ;;
  aarch64|arm64)       ARCH="arm64" ;;
  armv7l|armv8l|arm)   ARCH="arm" ;;
esac

# SCRIPT_DIR = директория, где лежит этот install.sh.
#   - Если вызывается как `sudo bash deploy/install.sh` из корня бандла
#     (старый layout) — SCRIPT_DIR = <bundle>/deploy, BUNDLE_DIR = <bundle>.
#   - Если вызывается как `sudo bash install.sh` из корня бандла
#     (v1.4.0 layout) — SCRIPT_DIR = BUNDLE_DIR = <bundle>.
#   - Если вызывается из dist/ (после `make build`) — SCRIPT_DIR = dist/,
#     BUNDLE_DIR = корень репозитория (для config.yml.sample).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# BUNDLE_DIR = корень архива.
#   - Если скрипт лежит в корне бандла (v1.4.0 layout: SCRIPT_DIR/install.sh,
#     SCRIPT_DIR/bin/...) — BUNDLE_DIR = SCRIPT_DIR.
#   - Если скрипт лежит в deploy/ (старый layout: deploy/install.sh,
#     ../awg-portal_x86-64) — BUNDLE_DIR = SCRIPT_DIR/..
#   - Если скрипт лежит в dist/ (после `make build`) — BUNDLE_DIR = SCRIPT_DIR.
if [ -d "${SCRIPT_DIR}/bin" ] || [ -f "${SCRIPT_DIR}/awg-portal_x86-64" ]; then
  BUNDLE_DIR="${SCRIPT_DIR}"
else
  BUNDLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

echo "==> awg-portal installer ${VERSION}"

if [ "$EUID" -ne 0 ]; then
  echo "ERROR: Run as root (sudo)."
  exit 1
fi

# Проверка зависимостей
for cmd in chown chmod cp id install mkdir systemctl useradd; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "ERROR: Required command '$cmd' not found."
    exit 1
  fi
done

# 1. awg-portal binary — выбор по архитектуре
#   Сначала ищем бинарник под архитектуру хоста, потом любой другой
echo "[1/5] Installing awg-portal..."
BINARY_SOURCE=""
# Строгий поиск: только под архитектуру хоста
for candidate in \
    "${BUNDLE_DIR}/bin/wg-portal-${ARCH}" \
    "${BUNDLE_DIR}/wg-portal-${ARCH}" \
    "${BUNDLE_DIR}/dist/wg-portal-${ARCH}"; do
  if [ -f "$candidate" ]; then
    BINARY_SOURCE="$candidate"
    break
  fi
done
# Fallback: любой бинарник (для обратной совместимости)
if [ -z "$BINARY_SOURCE" ]; then
  for candidate in \
      "${BUNDLE_DIR}/bin/wg-portal-amd64" \
      "${BUNDLE_DIR}/bin/wg-portal-arm64" \
      "${BUNDLE_DIR}/bin/wg-portal-arm" \
      "${BUNDLE_DIR}/wg-portal-amd64" \
      "${BUNDLE_DIR}/wg-portal-arm64" \
      "${BUNDLE_DIR}/wg-portal-arm" \
      "${BUNDLE_DIR}/dist/wg-portal" \
      "${BUNDLE_DIR}/dist/wg-portal-amd64" \
      "${BUNDLE_DIR}/dist/wg-portal-arm64" \
      "${BUNDLE_DIR}/dist/wg-portal-arm" \
      "${BUNDLE_DIR}/dist/awg-portal" \
      "${BUNDLE_DIR}/dist/awg-portal_x86-64" \
      "${BUNDLE_DIR}/awg-portal_x86-64" \
      "${BUNDLE_DIR}/awg-portal"; do
    if [ -f "$candidate" ]; then
      BINARY_SOURCE="$candidate"
      break
    fi
  done
fi
if [ -n "$BINARY_SOURCE" ]; then
  install -m 0755 "$BINARY_SOURCE" "${BIN_DIR}/awg-portal"
  echo "  Installed ${BIN_DIR}/awg-portal (from ${BINARY_SOURCE##*/}, arch=${ARCH})"
else
  echo "  ERROR: awg-portal binary not found in bundle."
  echo "  Searched: bin/, dist/, flat root — {wg-portal,awg-portal}{,-amd64,-arm64,-arm,_x86-64}"
  echo "  Build it: make build-amd64"
  exit 1
fi

# 2. amneziawg-go binary — выбор по архитектуре
#   Ищем amneziawg-go-{arch}, потом amneziawg-go (без суффикса)
echo "[2/5] Installing amneziawg-go..."
AWG_SOURCE=""
for candidate in \
    "${BUNDLE_DIR}/bin/amneziawg-go-${ARCH}" \
    "${BUNDLE_DIR}/bin/amneziawg-go"; do
  if [ -f "$candidate" ]; then
    AWG_SOURCE="$candidate"
    break
  fi
done
if [ -n "$AWG_SOURCE" ]; then
  install -m 0755 "$AWG_SOURCE" "${BIN_DIR}/amneziawg-go"
  echo "  Installed ${BIN_DIR}/amneziawg-go (from ${AWG_SOURCE##*/}, arch=${ARCH})"
else
  echo "  WARNING: amneziawg-go not found in bundle."
  echo "  AWG (обфускация) не будет работать. WG — будет."
  echo "  Установите позже в /usr/local/bin/amneziawg-go"
fi

# 3. Создаём системного пользователя (если нет)
echo "[3/5] Creating system user..."
if ! id -u "${SERVICE_USER}" &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
  echo "  User ${SERVICE_USER} created."
else
  echo "  User ${SERVICE_USER} already exists."
fi

# 4. Portal directories + конфиг
echo "[4/5] Setting up directories..."
mkdir -p "${PORTAL_DIR}/data"
mkdir -p "${PORTAL_DIR}/config" # для wg-конфигов (save-config)
mkdir -p /run/amneziawg          # для UAPI-сокетов AWG
chown -R "${SERVICE_USER}:${SERVICE_USER}" /run/amneziawg
chmod 0750 /run/amneziawg

# Auto-detect primary IP (source address of the default route).
# This is the IP clients will use to reach the portal, so we pre-fill
# external_url in config.yml so the user does not have to look it up.
# Falls back to "<IP>" placeholder if detection fails (e.g. no default route yet).
PRIMARY_IP=$(ip -4 route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[\d.]+' || echo "<IP>")
echo "  Detected primary IP: ${PRIMARY_IP}"

CONFIG_SOURCE=""
for candidate in \
    "${BUNDLE_DIR}/config.yml.sample" \
    "${BUNDLE_DIR}/config.yml" \
    "${SCRIPT_DIR}/../config.yml.sample" \
    "${SCRIPT_DIR}/config.yml.sample"; do
  if [ -f "$candidate" ]; then
    CONFIG_SOURCE="$candidate"
    break
  fi
done

if [ -n "$CONFIG_SOURCE" ]; then
  if [ ! -f "${PORTAL_DIR}/config.yml" ]; then
    cp "$CONFIG_SOURCE" "${PORTAL_DIR}/config.yml"
    # Replace placeholder IP in config
    sed -i "s|<IP>|${PRIMARY_IP}|g" "${PORTAL_DIR}/config.yml"
    # Also make sure external_url has http:// prefix
    sed -i "s|http://<IP>|http://${PRIMARY_IP}|g" "${PORTAL_DIR}/config.yml"
    chown "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}/config.yml"
    echo "  Sample config copied to ${PORTAL_DIR}/config.yml"
    echo "  ** REVIEW /opt/awg-portal/config.yml — change admin_password before starting! **"
  else
    echo "  Config already exists at ${PORTAL_DIR}/config.yml — not overwritten."
  fi
else
  echo "  WARNING: No config.yml.sample found — skipping config setup."
fi

# Права
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}"

# 5. Systemd
echo "[5/5] Installing systemd unit..."
cat > "${SYSTEMD_DIR}/awg-portal.service" << 'UNIT'
[Unit]
Description=AWG Portal — Web UI for WireGuard & AmneziaWG
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
RestrictAddressFamilies=AF_NETLINK AF_INET AF_INET6 AF_UNIX
RestrictRealtime=true
RestrictSUIDSGID=true
MemoryDenyWriteExecute=true
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload

echo ""
echo "==> Installation complete."
echo ""
echo "Installed:"
echo "  /usr/local/bin/awg-portal"
echo "  /usr/local/bin/amneziawg-go"
echo "  /opt/awg-portal/config.yml"
echo "  /etc/systemd/system/awg-portal.service"
echo ""
echo "Next steps:"
echo "  1. Edit /opt/awg-portal/config.yml — установите core.admin_user и core.admin_password"
echo "  2. systemctl enable --now awg-portal"
echo "  3. journalctl -u awg-portal -f"
echo ""
echo "If using AmneziaWG (обфускация):"
echo "  - Модуль amneziawg в ядре НЕ нужен — amneziawg-go работает в userspace"
echo "  - Требуется /dev/net/tun (проверьте: ls -la /dev/net/tun)"
echo "  - awg_mode: auto в config.yml выберет AWG только для интерфейсов с обфускацией"
echo "  - Если установлен kernel-модуль amneziawg — выгрузите:"
echo "      sudo modprobe -r amneziawg"
echo "      echo 'blacklist amneziawg' | sudo tee /etc/modprobe.d/blacklist-amneziawg.conf"