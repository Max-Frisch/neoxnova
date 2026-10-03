#!/usr/bin/env bash
# deploy-vm.sh — set up the niburu explorer on a fresh Debian/Ubuntu VM.
# Runs on the VM's own public IP (separate from the dev host) for a second
# account. No IP spoofing / header forgery is used.
#
# Usage on the VM (as a user with sudo):
#   git clone https://github.com/Max-Frisch/neoxnova.git
#   cd neoxnova/neoxnova/tools/explorer
#   ./deploy-vm.sh
#   # create ../../secrets/explorer.env with the SECOND account's credentials:
#   #   NIBURU_BASE_URL=https://niburuspace.com
#   #   NIBURU_USER=<second account>
#   #   NIBURU_PASS=<password>
#   EXPLORER_CHANNEL=chromium node explorer.mjs scan

set -euo pipefail
cd "$(dirname "$0")"

echo "[+] Installing Node.js LTS and Chromium..."
if ! command -v node >/dev/null 2>&1; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
  sudo apt-get install -y nodejs
fi
if ! command -v chromium >/dev/null 2>&1 && ! command -v chromium-browser >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y chromium || sudo apt-get install -y chromium-browser
fi

echo "[+] Installing explorer dependencies..."
npm install --no-audit --no-fund

CHROME="$(command -v chromium || command -v chromium-browser || true)"
echo "[+] Chromium at: ${CHROME:-NOT FOUND}"

cat <<EOF

[+] Done.
Set the second account's credentials in:
    $(cd ../.. && pwd)/secrets/explorer.env

Then run (use the system Chromium):
    EXPLORER_CHANNEL=chromium node explorer.mjs scan
    EXPLORER_CHANNEL=chromium node explorer.mjs status

If Chromium was not found, install snap chromium or set:
    EXPLORER_EXECUTABLE_PATH=/path/to/chrome node explorer.mjs scan
EOF
