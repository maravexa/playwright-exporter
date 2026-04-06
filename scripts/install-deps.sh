#!/usr/bin/env bash
# install-deps.sh — Install runtime dependencies for playwright-exporter.
# Supports: Ubuntu/Debian, RHEL/Fedora/CentOS/Rocky/AlmaLinux, Arch/Manjaro.
# Must be run as root.

set -euo pipefail

# ---------------------------------------------------------------------------
# Root check
# ---------------------------------------------------------------------------
if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: This script must be run as root (e.g. sudo ./scripts/install-deps.sh)" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Distro detection
# ---------------------------------------------------------------------------
if [[ ! -f /etc/os-release ]]; then
  echo "ERROR: /etc/os-release not found — cannot detect distro." >&2
  exit 1
fi

# shellcheck source=/dev/null
source /etc/os-release
DISTRO="${ID:-unknown}"

case "${DISTRO}" in
  ubuntu|debian)
    PKG_FAMILY="apt"
    ;;
  fedora|rhel|centos|rocky|almalinux)
    PKG_FAMILY="dnf"
    ;;
  arch|manjaro)
    PKG_FAMILY="pacman"
    ;;
  *)
    echo "ERROR: Unsupported distro '${DISTRO}'." >&2
    echo "Supported distros: ubuntu, debian, fedora, rhel, centos, rocky, almalinux, arch, manjaro" >&2
    exit 1
    ;;
esac

PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers
export PLAYWRIGHT_BROWSERS_PATH

# ---------------------------------------------------------------------------
# Helper: compare semver-style major version numbers
# Returns 0 (true) if installed node major >= MIN_NODE_MAJOR
# ---------------------------------------------------------------------------
MIN_NODE_MAJOR=18

node_meets_minimum() {
  if ! command -v node &>/dev/null; then
    return 1
  fi
  local ver
  ver=$(node --version 2>/dev/null | sed 's/^v//')
  local major
  major=$(echo "${ver}" | cut -d. -f1)
  [[ "${major}" -ge "${MIN_NODE_MAJOR}" ]]
}

# ---------------------------------------------------------------------------
# Step 1: Install Node.js
# ---------------------------------------------------------------------------
echo "[1/3] Installing Node.js..."

if node_meets_minimum; then
  NODE_VERSION=$(node --version)
  echo "  Node.js ${NODE_VERSION} already installed, skipping."
else
  case "${PKG_FAMILY}" in
    apt)
      apt-get update -y
      apt-get install -y curl ca-certificates gnupg
      # NodeSource LTS (Node 22 at time of writing)
      curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
      apt-get install -y nodejs
      ;;
    dnf)
      dnf install -y curl ca-certificates
      curl -fsSL https://rpm.nodesource.com/setup_lts.x | bash -
      dnf install -y nodejs
      ;;
    pacman)
      pacman -Sy --noconfirm nodejs-lts-iron npm
      ;;
  esac
  echo "  Node.js $(node --version) installed."
fi

# ---------------------------------------------------------------------------
# Step 2: Verify npm
# ---------------------------------------------------------------------------
echo "[2/3] Verifying npm..."

if ! command -v npm &>/dev/null; then
  echo "  npm not found after Node.js install — attempting to install separately..."
  case "${PKG_FAMILY}" in
    apt)   apt-get install -y npm ;;
    dnf)   dnf install -y npm ;;
    pacman) pacman -S --noconfirm npm ;;
  esac
fi

NPM_VERSION=$(npm --version)
echo "  npm v${NPM_VERSION} present."

# ---------------------------------------------------------------------------
# Create system user and directories
# ---------------------------------------------------------------------------
if ! id playwright-exporter &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin playwright-exporter
  echo "  Created system user: playwright-exporter"
else
  echo "  System user playwright-exporter already exists, skipping."
fi

mkdir -p "${PLAYWRIGHT_BROWSERS_PATH}" /opt/playwright-tests
chown playwright-exporter:playwright-exporter "${PLAYWRIGHT_BROWSERS_PATH}" /opt/playwright-tests
echo "  Directories ready: ${PLAYWRIGHT_BROWSERS_PATH}, /opt/playwright-tests"

# ---------------------------------------------------------------------------
# Step 3: Install Playwright Chromium
# ---------------------------------------------------------------------------
echo "[3/3] Installing Playwright Chromium..."

# Check if Playwright is already installed with Chromium present.
PLAYWRIGHT_ALREADY_INSTALLED=false
if command -v npx &>/dev/null && \
   PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH}" npx --yes playwright --version &>/dev/null; then
  # Check for an actual chromium directory under the browsers path.
  if ls "${PLAYWRIGHT_BROWSERS_PATH}"/chromium-* &>/dev/null 2>&1; then
    PW_VERSION=$(PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH}" npx playwright --version 2>/dev/null | awk '{print $NF}')
    echo "  Playwright ${PW_VERSION} with Chromium already installed at ${PLAYWRIGHT_BROWSERS_PATH}, skipping."
    PLAYWRIGHT_ALREADY_INSTALLED=true
  fi
fi

if [[ "${PLAYWRIGHT_ALREADY_INSTALLED}" == "false" ]]; then
  PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH}" \
    npx --yes playwright install --with-deps chromium
  # Fix ownership so the service user can read the browsers.
  chown -R playwright-exporter:playwright-exporter "${PLAYWRIGHT_BROWSERS_PATH}"
  echo "  Playwright Chromium installed to ${PLAYWRIGHT_BROWSERS_PATH}."
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
NODE_VER=$(node --version)
NPM_VER=$(npm --version)
PW_VER=$(PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH}" npx playwright --version 2>/dev/null | awk '{print $NF}')
CHROMIUM_DIR=$(ls -d "${PLAYWRIGHT_BROWSERS_PATH}"/chromium-* 2>/dev/null | head -1 || echo "(not found)")

cat <<EOF

=== Installation Complete ===
  Node.js:    ${NODE_VER}
  npm:        v${NPM_VER}
  Playwright: ${PW_VER}
  Chromium:   ${CHROMIUM_DIR}

Next steps:
  1. Copy the exporter binary to /usr/local/bin/playwright-exporter
  2. Copy your config to /etc/playwright-exporter/config.yaml
  3. Place Playwright test directories under /opt/playwright-tests/
  4. Ensure your systemd unit includes: Environment=PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers
  5. Start the service: systemctl enable --now playwright-exporter
EOF
