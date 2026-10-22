#!/bin/bash
set -euo pipefail

# Step 1: Create system user
if ! id -u playwright-exporter &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin playwright-exporter
fi

# Step 2: Create only missing directories. Never recursively change operator data.
for dir in /opt/playwright-tests /opt/playwright-browsers /tmp/playwright-exporter; do
    if [[ -e "$dir" || -L "$dir" ]]; then
        if [[ ! -d "$dir" || -L "$dir" ]]; then
            echo "ERROR: inspect existing non-directory or symlink at $dir; nothing was replaced." >&2
            exit 1
        fi
        # Existing ownership, permissions and all contents remain operator-managed.
        continue
    fi
    install -d -m 0755 -o playwright-exporter -g playwright-exporter "$dir"
done

# Step 3: Reload systemd
systemctl daemon-reload

echo ""
echo "=== playwright-exporter package installed ==="
echo ""
echo "Run the setup script to install Playwright and Chromium:"
echo ""
echo "    sudo playwright-exporter-setup"
echo ""
