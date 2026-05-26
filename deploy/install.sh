#!/bin/bash
# awg-portal installation script
# Run as root: sudo bash install.sh

set -euo pipefail

BINARY="${1:-./wg-portal-amd64}"
SYSTEMD_DIR="/etc/systemd/system"
BIN_DIR="/usr/local/bin"

echo "==> awg-portal installer"
echo ""

if [ "$EUID" -ne 0 ]; then
  echo "ERROR: Please run as root (sudo)."
  exit 1
fi

# --- Binary ---
if [ ! -f "$BINARY" ]; then
  echo "ERROR: Binary not found: $BINARY"
  exit 1
fi

echo "[1/5] Installing binary to ${BIN_DIR}/wg-portal..."
install -m 0755 "$BINARY" "${BIN_DIR}/wg-portal"

# --- Build amneziawg-go ---
echo "[2/5] Building amneziawg-go..."
if command -v go &>/dev/null; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  cd "$SCRIPT_DIR"
  if [ -d ../amneziawg-go ]; then
    cd ../amneziawg-go
    CGO_ENABLED=0 go build -o /usr/local/bin/amneziawg-go -tags netgo .
    install -m 0755 /usr/local/bin/amneziawg-go "${BIN_DIR}/amneziawg-go"
  else
    echo "  WARNING: amneziawg-go source not found. Build manually."
  fi
else
  echo "  WARNING: Go not found, skipping amneziawg-go build."
fi

# --- Systemd units ---
echo "[3/5] Installing systemd units..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -f "${SCRIPT_DIR}/amneziawg@.service" ]; then
  install -m 0644 "${SCRIPT_DIR}/amneziawg@.service" "${SYSTEMD_DIR}/amneziawg@.service"
fi
if [ -f "${SCRIPT_DIR}/wg-portal.service" ]; then
  install -m 0644 "${SCRIPT_DIR}/wg-portal.service" "${SYSTEMD_DIR}/wg-portal.service"
fi

# Override for wg-portal -> amneziawg dependency
OVERRIDE_DIR="${SYSTEMD_DIR}/wg-portal.service.d"
mkdir -p "$OVERRIDE_DIR"
if [ -f "${SCRIPT_DIR}/override-wg-portal.conf" ]; then
  install -m 0644 "${SCRIPT_DIR}/override-wg-portal.conf" "${OVERRIDE_DIR}/override.conf"
fi

# --- Reload and enable ---
echo "[4/5] Reloading systemd..."
systemctl daemon-reload

echo "[5/5] Enabling amneziawg@wg0..."
systemctl enable amneziawg@wg0 || echo "  WARNING: enable failed (interface wg0 may not exist yet)"

echo ""
echo "==> Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Create interface: systemctl start amneziawg@wg0"
echo "  2. Start portal:     systemctl start wg-portal"
echo "  3. Check:            systemctl status amneziawg@wg0 wg-portal"
echo ""
echo "Config: /etc/wg-portal/config.yml"
echo "Docs:   https://github.com/h44z/wg-portal"
