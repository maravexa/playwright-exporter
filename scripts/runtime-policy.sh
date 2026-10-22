#!/usr/bin/env bash
# Pure policy helpers: sourcing this file performs no installation or system changes.

runtime_node_supported() {
    local version=${1#v}
    [[ "$version" =~ ^(22|24)\.[0-9]+\.[0-9]+$ ]]
}

runtime_check_node() {
    local version
    if ! version=$(node --version 2>/dev/null) || ! runtime_node_supported "$version"; then
        echo "ERROR: provision a maintained Node.js 22.x or 24.x release using your trusted package source." >&2
        return 1
    fi
    if ! command -v npm >/dev/null 2>&1; then
        echo "ERROR: provision npm alongside Node.js before running setup." >&2
        return 1
    fi
}
