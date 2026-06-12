#!/bin/bash
# awg-portal installation script
# Run as root: bash deploy/install.sh (from bundle root)
set -euo pipefail

VERSION="v1.4.0"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/awg-portal"
SERVICE_USER="awg-portal"

# SCRIPT_DIR = директория где лежит install.sh (deploy/)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# BUNDLE_DIR = корень архива (родитель deploy/)
BUNDLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

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

# 1. awg-portal binary
echo "[1/5] Installing awg-portal..."
BINARY_SOURCE=""
# Поиск по приоритету: dist/ (после make build*) → корень бандла (back compat)
# Имя в /usr/local/bin всегда awg-portal независимо от исходного имени.
for candidate in \
    "${BUNDLE_DIR}/dist/awg-portal_x86-64" \
    "${BUNDLE_DIR}/dist/wg-portal" \
    "${BUNDLE_DIR}/dist/wg-portal-amd64" \
    "${BUNDLE_DIR}/dist/wg-portal-arm64" \
    "${BUNDLE_DIR}/dist/wg-portal-arm" \
    "${BUNDLE_DIR}/dist/awg-portal" \
    "${BUNDLE_DIR}/awg-portal_x86-64" \
    "${BUNDLE_DIR}/awg-portal"; do
  if [ -f "$candidate" ]; then
    BINARY_SOURCE="$candidate"
    break
  fi
done
if [ -n "$BINARY_SOURCE" ]; then
  install -m 0755 "$BINARY_SOURCE" "${BIN_DIR}/awg-portal"
  echo "  Installed ${BIN_DIR}/awg-portal (from ${BINARY_SOURCE##*/})"
else
  echo "  ERROR: awg-portal binary not found in bundle."
  echo "  Searched: dist/{awg-portal_x86-64,wg-portal,wg-portal-amd64,wg-portal-arm64,wg-portal-arm,awg-portal}, {awg-portal_x86-64,awg-portal}"
  echo "  Build it: make build-amd64"
  exit 1
fi

# 2. amneziawg-go binary (bundled)
echo "[2/5] Installing amneziawg-go..."
AWG_SOURCE=""
for candidate in \
    "${BUNDLE_DIR}/dist/amneziawg-go" \
    "${BUNDLE_DIR}/amneziawg-go" \
    "${SCRIPT_DIR}/../amneziawg-go"; do
  if [ -f "$candidate" ]; then
    AWG_SOURCE="$candidate"
    break
  fi
done
if [ -n "$AWG_SOURCE" ]; then
  install -m 0755 "$AWG_SOURCE" "${BIN_DIR}/amneziawg-go"
else
  echo "  WARNING: amneziawg-go not found in bundle."
  echo "  AWG (обфускация) не будет работать. WG — будет."
  echo "  Установите позже: wget .../amneziawg-go"
fi

# 3. Создаём системного пользователя (если нет)
echo "[3/5] Creating system user..."
if ! id -u "${SERVICE_USER}" &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
  echo "  User ${SERVICE_USER} created."
fi

# 4. Portal directories
echo "[4/5] Setting up directories..."
mkdir -p "${PORTAL_DIR}/data"
mkdir -p "${PORTAL_DIR}/config" # для wg-конфигов (save-config)
mkdir -p /run/amneziawg # для UAPI-сокетов AWG
chown -R "${SERVICE_USER}:${SERVICE_USER}" /run/amneziawg
chmod 0750 /run/amneziawg

CONFIG_SOURCE=""
for candidate in "${BUNDLE_DIR}/config.yml.sample" "${BUNDLE_DIR}/config.yml" "${SCRIPT_DIR}/../config.yml.sample"; do
  if [ -f "$candidate" ]; then
    CONFIG_SOURCE="$candidate"
    break
  fi
done

if [ -n "$CONFIG_SOURCE" ]; then
  if [ ! -f "${PORTAL_DIR}/config.yml" ]; then
    cp "$CONFIG_SOURCE" "${PORTAL_DIR}/config.yml"
    echo "  Sample config copied to ${PORTAL_DIR}/config.yml"
    echo "  ** EDIT THIS FILE before starting the service! **"
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
AmbientCapabilities=CAP_NET_ADMIN
Restart=on-failure
RestartSec=3
StartLimitBurst=5
WorkingDirectory=/opt/awg-portal
Environment=WG_PORTAL_CONFIG=/opt/awg-portal/config.yml
ExecStart=/usr/local/bin/awg-portal
RuntimeDirectory=amneziawg

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
CapabilityBoundingSet=CAP_NET_ADMIN

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
