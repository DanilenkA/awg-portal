#!/bin/bash
# awg-portal installation script — v1.0.0
# Run as root: sudo bash install.sh
set -euo pipefail

BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"
PORTAL_DIR="/opt/wg-portal"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> awg-portal installer v1.0.0"

if [ "$EUID" -ne 0 ]; then
  echo "ERROR: Run as root (sudo)."
  exit 1
fi

# 1. Binary
echo "[1/4] Installing wg-portal binary..."
if [ -f "${SCRIPT_DIR}/wg-portal-amd64" ]; then
  install -m 0755 "${SCRIPT_DIR}/wg-portal-amd64" "${BIN_DIR}/wg-portal"
elif [ -f "${SCRIPT_DIR}/../dist/wg-portal-amd64" ]; then
  install -m 0755 "${SCRIPT_DIR}/../dist/wg-portal-amd64" "${BIN_DIR}/wg-portal"
else
  echo "  WARNING: wg-portal-amd64 not found, skip."
fi

# 2. amneziawg-go
echo "[2/4] Checking amneziawg-go..."
if command -v amneziawg-go &>/dev/null; then
  echo "  amneziawg-go found at $(which amneziawg-go)"
else
  echo "  WARNING: amneziawg-go not found in PATH."
  echo "  Build manually:"
  echo "    git clone https://github.com/amnezia-vpn/amneziawg-go"
  echo "    cd amneziawg-go && CGO_ENABLED=0 go build -o /usr/local/bin/amneziawg-go -tags netgo ."
fi

# 3. Portal directory and config
echo "[3/4] Setting up ${PORTAL_DIR}..."
mkdir -p "${PORTAL_DIR}"
if [ ! -f "${PORTAL_DIR}/config.yml" ]; then
  if [ -f "${SCRIPT_DIR}/../config.yml.sample" ]; then
    cp "${SCRIPT_DIR}/../config.yml.sample" "${PORTAL_DIR}/config.yml"
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
echo "Next steps:"
echo "  1. Edit /opt/wg-portal/config.yml"
echo "  2. Ensure amneziawg-go is in PATH"
echo "  3. systemctl enable --now wg-portal"
echo "  4. journalctl -u wg-portal -f"
