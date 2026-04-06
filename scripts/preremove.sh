#!/bin/bash
set -euo pipefail

# Stop and disable the service if running
if systemctl is-active --quiet playwright-exporter; then
    systemctl stop playwright-exporter
fi
if systemctl is-enabled --quiet playwright-exporter 2>/dev/null; then
    systemctl disable playwright-exporter
fi
