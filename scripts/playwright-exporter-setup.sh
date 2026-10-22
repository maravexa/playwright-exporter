#!/usr/bin/env bash
# Explicit setup only. Never changes operator projects, package sources, or OS packages.
set -euo pipefail
umask 022

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
if [[ -f "$SCRIPT_DIR/runtime-policy.sh" ]]; then
    POLICY_DIR=$SCRIPT_DIR
    MANIFEST_DIR=$SCRIPT_DIR/../runtime
else
    POLICY_DIR=/usr/share/playwright-exporter/runtime
    MANIFEST_DIR=$POLICY_DIR
fi
# shellcheck source=runtime-policy.sh
source "$POLICY_DIR/runtime-policy.sh"

if [[ $EUID -ne 0 ]]; then
    echo "ERROR: run setup as root after reviewing its locked runtime and prerequisites." >&2
    exit 1
fi
runtime_check_node
if ! id -u playwright-exporter >/dev/null 2>&1; then
    echo "ERROR: provision the playwright-exporter service account before setup." >&2
    exit 1
fi
NODE_BIN=$(command -v node)
if ! runuser -u playwright-exporter -- "$NODE_BIN" --version >/dev/null; then
    echo "ERROR: the operator-provided Node binary must be accessible to playwright-exporter." >&2
    exit 1
fi
if [[ ! -d /opt/playwright-browsers || -L /opt/playwright-browsers ]] ||
   ! runuser -u playwright-exporter -- test -w /opt/playwright-browsers; then
    echo "ERROR: provision a real /opt/playwright-browsers directory writable by playwright-exporter first." >&2
    exit 1
fi

# Read the version from the reviewed manifest; never resolve a registry tag.
PLAYWRIGHT_VERSION=$(node -p 'require(process.argv[1]).dependencies["@playwright/test"]' "$MANIFEST_DIR/package.json")
if [[ ! "$PLAYWRIGHT_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: runtime manifest must specify an exact Playwright version." >&2
    exit 1
fi
RUNTIME_PARENT=/opt/playwright-exporter
RUNTIME_DIR=$RUNTIME_PARENT/runtime-$PLAYWRIGHT_VERSION
if [[ -L "$RUNTIME_PARENT" ]]; then
    echo "ERROR: runtime parent must not be a symlink." >&2
    exit 1
fi
if [[ -e "$RUNTIME_PARENT" ]] && [[ $(stat -c '%u:%a' "$RUNTIME_PARENT") != '0:755' ]]; then
    echo "ERROR: existing runtime parent must be root-owned with mode 0755." >&2
    exit 1
fi
install -d -m 0755 "$RUNTIME_PARENT"
exec 9>"$RUNTIME_PARENT/.setup.lock"
if ! flock -n 9; then
    echo "ERROR: another setup is running; retry after it completes." >&2
    exit 1
fi
if [[ -e "$RUNTIME_DIR" || -L "$RUNTIME_DIR" ]]; then
    # Do not overwrite an existing runtime, including an unknown or partial one.
    if [[ -L "$RUNTIME_DIR" ]] ||
       [[ $(stat -c '%u:%a' "$RUNTIME_DIR") != '0:755' ]] ||
       ! cmp -s "$MANIFEST_DIR/package.json" "$RUNTIME_DIR/package.json" ||
       ! cmp -s "$MANIFEST_DIR/package-lock.json" "$RUNTIME_DIR/package-lock.json" ||
       [[ ! -f "$RUNTIME_DIR/node_modules/playwright/cli.js" ]]; then
        echo "ERROR: existing runtime differs or is incomplete; inspect it manually. Nothing was replaced." >&2
        exit 1
    fi
else
    STAGING_DIR=$(mktemp -d "$RUNTIME_PARENT/.runtime-XXXXXXXX")
    trap 'if [[ -n "${STAGING_DIR:-}" ]]; then rm -rf -- "$STAGING_DIR"; fi' EXIT
    install -m 0644 "$MANIFEST_DIR/package.json" "$MANIFEST_DIR/package-lock.json" "$STAGING_DIR/"
    # npm verifies lockfile integrity; package lifecycle scripts and audit calls are disabled.
    (cd "$STAGING_DIR" && npm ci --engine-strict --ignore-scripts --no-audit --no-fund --cache "$STAGING_DIR/.npm-cache")
    rm -rf -- "$STAGING_DIR/.npm-cache"
    chmod 0755 "$STAGING_DIR"
    mv -T -- "$STAGING_DIR" "$RUNTIME_DIR"
    STAGING_DIR=
fi

# OS browser libraries are operator-provided. Never invoke install-deps or --with-deps.
# Browser download runs as the service user, using the root-owned locked CLI.
runuser -u playwright-exporter -- env PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers \
    "$NODE_BIN" "$RUNTIME_DIR/node_modules/playwright/cli.js" install chromium

echo "Locked Playwright $PLAYWRIGHT_VERSION runtime prepared at $RUNTIME_DIR."
echo "Operator projects were not initialized, installed, upgraded, or rewritten."
echo "Provision matching locked project dependencies and OS browser libraries separately."
echo "Use each project's ./node_modules/.bin/playwright in schedule commands."
