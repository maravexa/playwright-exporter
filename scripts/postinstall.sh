#!/bin/bash
set -euo pipefail

# Step 1: Create system user
if ! id -u playwright-exporter &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin playwright-exporter
fi

# Step 2: Create and set directory ownership
mkdir -p /opt/playwright-tests
mkdir -p /opt/playwright-browsers
mkdir -p /tmp/playwright-exporter/npm-cache
chown -R playwright-exporter:playwright-exporter /opt/playwright-tests
chown -R playwright-exporter:playwright-exporter /opt/playwright-browsers
chown -R playwright-exporter:playwright-exporter /tmp/playwright-exporter

# Step 3: Reload systemd
systemctl daemon-reload

echo ""
echo "=== playwright-exporter package installed ==="
echo ""
echo "Run the setup script to install Playwright and Chromium:"
echo ""
echo "    sudo playwright-exporter-setup"
echo ""
