#!/bin/bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# Step 1: Create system user
if ! id -u playwright-exporter &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin playwright-exporter
fi

# Step 2: Set directory ownership
chown -R playwright-exporter:playwright-exporter /opt/playwright-tests
chown -R playwright-exporter:playwright-exporter /opt/playwright-browsers
chown -R playwright-exporter:playwright-exporter /tmp/playwright-exporter

# Step 3: Install Playwright Chromium to shared path
# Check node version and warn if too old, but don't fail (package manager dep handles it)
NODE_VERSION=$(node --version 2>/dev/null | sed 's/v//' | cut -d. -f1 || echo "0")
if [ "${NODE_VERSION}" -lt 18 ] 2>/dev/null; then
    echo "WARNING: Node.js version $(node --version 2>/dev/null || echo 'not found') is older than v18." >&2
    echo "WARNING: playwright-exporter requires Node.js v18+. Please upgrade Node.js." >&2
    echo "WARNING: On some systems you may need to add the NodeSource repository." >&2
fi

# Install OS-level Chromium dependencies (requires root)
DEBIAN_FRONTEND=noninteractive npx --yes playwright install-deps chromium

# Install Chromium browser binary as service user (avoids root-owned files)
cd /tmp
runuser -s /bin/bash -c 'PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers npm_config_cache=/tmp/playwright-exporter/npm-cache npx --yes playwright install chromium' playwright-exporter

# Step 4: Initialize test directories with @playwright/test
for dir in /opt/playwright-tests/*/; do
    [ -d "$dir" ] || continue
    if [ ! -d "${dir}node_modules/@playwright/test" ]; then
        echo "Initializing Playwright in ${dir}..."
        cd "$dir"
        # Only run npm init if no package.json exists
        [ -f "package.json" ] || runuser -s /bin/bash -c 'npm_config_cache=/tmp/playwright-exporter/npm-cache npm init -y' playwright-exporter
        runuser -s /bin/bash -c 'npm_config_cache=/tmp/playwright-exporter/npm-cache npm install @playwright/test' playwright-exporter
        chown -R playwright-exporter:playwright-exporter "$dir"
    fi
done

# Step 5: Reload systemd and print status
systemctl daemon-reload

echo ""
echo "=== playwright-exporter installed ==="
echo "  Config: /etc/playwright-exporter/config.yaml"
echo "  Tests:  /opt/playwright-tests/"
echo ""
echo "Next steps:"
echo "  1. Edit /etc/playwright-exporter/config.yaml with your schedules"
echo "  2. Place Playwright test directories under /opt/playwright-tests/"
echo "  3. Start the service: systemctl enable --now playwright-exporter"
echo ""
