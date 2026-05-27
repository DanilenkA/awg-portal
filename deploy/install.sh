#!/bin/bash
# awg-portal installation script — v1.0.1
# Run as root: sudo bash install.sh
set -euo pipefail

BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/wg-portal"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> awg-portal installer v1.0.1"

if [ "$EUID" -ne 0 ]; then
  echo "ERROR: Run as root (sudo)."
  exit 1
fi

# 1. wg-portal binary
echo "[1/4] Installing wg-portal..."
if [ -f "${SCRIPT_DIR}/wg-portal-amd64" ]; then
  install -m 0755 "${SCRIPT_DIR}/wg-portal-amd64" "${BIN_DIR}/wg-portal"
elif [ -f "${SCRIPT_DIR}/../dist/wg-portal-amd64" ]; then
  install -m 0755 "${SCRIPT_DIR}/../dist/wg-portal-amd64" "${BIN_DIR}/wg-portal"
else
  echo "  ERROR: wg-portal-amd64 not found in the bundle."
  exit 1
fi

# 2. amneziawg-go binary (bundled)
echo "[2/4] Installing amneziawg-go..."
if [ -f "${SCRIPT_DIR}/amneziawg-go" ]; then
  install -m 0755 "${SCRIPT_DIR}/amneziawg-go" "${BIN_DIR}/amneziawg-go"
elif [ -f "${SCRIPT_DIR}/../dist/amneziawg-go" ]; then
  install -m 0755 "${SCRIPT_DIR}/../dist/amneziawg-go" "${BIN_DIR}/amneziawg-go"
else
  echo "  ERROR: amneziawg-go not found in the bundle."
  echo "  Build manually: cd amneziawg-go && CGO_ENABLED=0 go build -tags netgo ."
  exit 1
fi

# 3. Portal directory and config
echo "[3/4] Setting up ${PORTAL_DIR}..."
mkdir -p "${PORTAL_DIR}"
if [ ! -f "${PORTAL_DIR}/config.yml" ]; then
  if [ -f "${SCRIPT_DIR}/config.yml.sample" ]; then
    cp "${SCRIPT_DIR}/config.yml.sample" "${PORTAL_DIR}/config.yml"
  elif [ -f "${SCRIPT_DIR}/../wg-portal/config.yml.sample" ]; then
    cp "${SCRIPT_DIR}/../wg-portal/config.yml.sample" "${PORTAL_DIR}/config.yml"
  fi
  echo "  Sample config copied. EDIT ${PORTAL_DIR}/config.yml before starting!"
fi

# 4. Systemd
echo "[4/4] Installing systemd unit..."
cat > "${SYSTEMD_DIR}/wg-portal.service" << 'UNIT'
[Unit]
Description=WireGuard Portal with AmneziaWG
After=network.target

[Service]
Type=simple
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
Restart=on-failure
RestartSec=10
WorkingDirectory=/opt/wg-portal
Environment=WG_PORTAL_CONFIG=/opt/wg-portal/config.yml
ExecStart=/usr/local/bin/wg-portal

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload

echo ""
echo "==> Installation complete."
echo ""
echo "Installed:"
echo "  /usr/local/bin/wg-portal   ($(file /usr/local/bin/wg-portal | cut -d, -f2-))"
echo "  /usr/local/bin/amneziawg-go ($(file /usr/local/bin/amneziawg-go | cut -d, -f2-))"
echo ""
echo "Next steps:"
echo "  1. Edit /opt/wg-portal/config.yml"
echo "  2. systemctl enable --now wg-portal"
echo "  3. journalctl -u wg-portal -f"
