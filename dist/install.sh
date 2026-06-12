#!/bin/bash
# awg-portal installation script — v1.4.0
#
# Run after `make build` / `make build-amd64`. This script is the
# single-file copy of deploy/install.sh placed at dist/install.sh so
# `cd dist && sudo bash install.sh` works after a local build.
#
# Run as root: sudo bash install.sh
set -euo pipefail

VERSION="v1.4.0"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/awg-portal"
SERVICE_USER="awg-portal"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# При запуске из dist/ — это корень репозитория (для config.yml.sample).
# ВАЖНО: не "подниматься выше" если мы уже в корне бандла/репо.
if [ -d "${SCRIPT_DIR}/bin" ] || [ -f "${SCRIPT_DIR}/awg-portal_x86-64" ] || [ -f "${SCRIPT_DIR}/wg-portal-amd64" ]; then
  BUNDLE_DIR="${SCRIPT_DIR}"
else
  BUNDLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

echo "==> awg-portal installer ${VERSION}"

if [ "$EUID" -ne 0 ]; then
  echo "ERROR: Run as root (sudo)."
  exit 1
fi

# 1. awg-portal binary — ищем в SCRIPT_DIR (dist/), dist/bin/ и BUNDLE_DIR.
echo "[1/5] Installing awg-portal..."
BINARY_SOURCE=""
for candidate in \
    "${SCRIPT_DIR}/bin/wg-portal-amd64" \
    "${SCRIPT_DIR}/bin/wg-portal-arm64" \
    "${SCRIPT_DIR}/bin/wg-portal-arm" \
    "${SCRIPT_DIR}/wg-portal-amd64" \
    "${SCRIPT_DIR}/wg-portal-arm64" \
    "${SCRIPT_DIR}/wg-portal-arm" \
    "${SCRIPT_DIR}/wg-portal" \
    "${SCRIPT_DIR}/awg-portal_x86-64" \
    "${SCRIPT_DIR}/awg-portal" \
    "${BUNDLE_DIR}/dist/wg-portal-amd64" \
    "${BUNDLE_DIR}/dist/wg-portal" \
    "${BUNDLE_DIR}/dist/awg-portal_x86-64"; do
  if [ -f "$candidate" ]; then
    BINARY_SOURCE="$candidate"
    break
  fi
done
if [ -n "$BINARY_SOURCE" ]; then
  install -m 0755 "$BINARY_SOURCE" "${BIN_DIR}/awg-portal"
  echo "  Installed ${BIN_DIR}/awg-portal (from ${BINARY_SOURCE})"
else
  echo "  ERROR: awg-portal binary not found."
  echo "  Searched: dist/{bin/,}wg-portal-amd64, dist/wg-portal, dist/awg-portal_x86-64"
  echo "  Build it: make build-amd64"
  exit 1
fi

# 2. amneziawg-go binary (bundled)
echo "[2/5] Installing amneziawg-go..."
AWG_SOURCE=""
for candidate in \
    "${SCRIPT_DIR}/bin/amneziawg-go" \
    "${SCRIPT_DIR}/amneziawg-go" \
    "${BUNDLE_DIR}/dist/amneziawg-go"; do
  if [ -f "$candidate" ]; then
    AWG_SOURCE="$candidate"
    break
  fi
done
if [ -n "$AWG_SOURCE" ]; then
  install -m 0755 "$AWG_SOURCE" "${BIN_DIR}/amneziawg-go"
  echo "  Installed ${BIN_DIR}/amneziawg-go (from ${AWG_SOURCE})"
else
  echo "  WARNING: amneziawg-go not found in the bundle."
  echo "  AWG (обфускация) не будет работать. WG — будет."
  echo "  Build manually: cd amneziawg-go && CGO_ENABLED=0 go build -tags netgo ."
fi

# 3. Создаём системного пользователя (если нет) и /run/amneziawg.
echo "[3/5] Creating system user and runtime dirs..."
if ! id -u "${SERVICE_USER}" &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
  echo "  User ${SERVICE_USER} created."
else
  echo "  User ${SERVICE_USER} already exists."
fi
mkdir -p /run/amneziawg
chown -R "${SERVICE_USER}:${SERVICE_USER}" /run/amneziawg
chmod 0750 /run/amneziawg

# 4. Portal directory and config
echo "[4/5] Setting up ${PORTAL_DIR}..."
mkdir -p "${PORTAL_DIR}/data"
mkdir -p "${PORTAL_DIR}/config"
CONFIG_SOURCE=""
for candidate in \
    "${SCRIPT_DIR}/config.yml.sample" \
    "${BUNDLE_DIR}/config.yml.sample" \
    "${BUNDLE_DIR}/wg-portal/config.yml.sample"; do
  if [ -f "$candidate" ]; then
    CONFIG_SOURCE="$candidate"
    break
  fi
done
if [ -n "$CONFIG_SOURCE" ]; then
  if [ ! -f "${PORTAL_DIR}/config.yml" ]; then
    cp "$CONFIG_SOURCE" "${PORTAL_DIR}/config.yml"
    chown "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}/config.yml"
    echo "  Sample config copied to ${PORTAL_DIR}/config.yml"
    echo "  ** EDIT THIS FILE before starting the service! **"
  else
    echo "  Config already exists at ${PORTAL_DIR}/config.yml — not overwritten."
  fi
else
  echo "  WARNING: No config.yml.sample found — skipping config setup."
fi
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${PORTAL_DIR}"

# 5. Systemd
echo "[5/5] Installing systemd unit..."
# Используем полный unit из dist/wg-portal.service или scripts/wg-portal.service.
SERVICE_SOURCE=""
for candidate in \
    "${SCRIPT_DIR}/wg-portal.service" \
    "${SCRIPT_DIR}/../scripts/wg-portal.service" \
    "${SCRIPT_DIR}/../wg-portal/scripts/wg-portal.service"; do
  if [ -f "$candidate" ]; then
    SERVICE_SOURCE="$candidate"
    break
  fi
done
if [ -n "$SERVICE_SOURCE" ]; then
  install -m 0644 "$SERVICE_SOURCE" "${SYSTEMD_DIR}/awg-portal.service"
else
  echo "  WARNING: wg-portal.service not found in bundle — using minimal unit."
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
fi

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