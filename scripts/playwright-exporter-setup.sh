#!/bin/bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# Must be root
if [ "$EUID" -ne 0 ]; then
    echo "Error: must be run as root (sudo playwright-exporter-setup)" >&2
    exit 1
fi

echo "[1/4] Checking Node.js version..."
NODE_VERSION=$(node --version 2>/dev/null | sed 's/v//' | cut -d. -f1 || echo "0")
if [ "${NODE_VERSION}" -lt 18 ] 2>/dev/null; then
    echo "WARNING: Node.js version $(node --version 2>/dev/null || echo 'not found') is older than v18." >&2
    echo "WARNING: playwright-exporter requires Node.js v18+. Please upgrade Node.js." >&2
fi

echo "[2/4] Installing Chromium OS dependencies..."
cd /tmp
DEBIAN_FRONTEND=noninteractive npx --yes playwright install-deps chromium

echo "[3/4] Installing Chromium browser binary..."
runuser -s /bin/bash -c 'PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers npm_config_cache=/tmp/playwright-exporter/npm-cache npx --yes playwright install chromium' playwright-exporter

echo "[4/4] Initializing Playwright test directories..."
for dir in /opt/playwright-tests/*/; do
    [ -d "$dir" ] || continue
    if [ ! -d "${dir}node_modules/@playwright/test" ]; then
        echo "  Initializing ${dir}..."
        cd "$dir"
        [ -f "package.json" ] || runuser -s /bin/bash -c 'npm_config_cache=/tmp/playwright-exporter/npm-cache npm init -y' playwright-exporter
        runuser -s /bin/bash -c 'npm_config_cache=/tmp/playwright-exporter/npm-cache npm install @playwright/test' playwright-exporter
    fi
done

echo ""
echo "=== Setup Complete ==="
echo "  Node.js:    $(node --version 2>/dev/null || echo 'not found')"
echo "  npm:        $(npm --version 2>/dev/null || echo 'not found')"
echo "  Chromium:   $(ls -d /opt/playwright-browsers/chromium-* 2>/dev/null || echo 'not found')"
echo ""
echo "Next steps:"
echo "  1. Edit /etc/playwright-exporter/config.yaml with your schedules"
echo "  2. Place Playwright test directories under /opt/playwright-tests/"
echo "  3. Start the service: systemctl enable --now playwright-exporter"
echo ""
