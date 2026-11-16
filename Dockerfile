# The digest pins the upstream Node/browser/OS baseline. Public APT updates and
# a pinned npm CLI are applied below; the final image must pass its advisory gate.
FROM mcr.microsoft.com/playwright:v1.64.1-resolute@sha256:f76885029244e5a384243a930ff283cef67d90653978ee27a77dd4cff8882fb4

ARG NPM_VERSION=12.2.2

# This application uses Tini, not the upstream image's unpackaged Pebble helper.
# Remove it from the merged runtime filesystem; pinned lower layers retain it.
RUN rm -- /usr/bin/pebble

# Use the image's existing signed repositories. Do not add keys or repositories.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends --only-upgrade apt libapt-pkg7.0 nodejs \
    && apt-get install --yes --no-install-recommends tini \
    && rm -rf /var/lib/apt/lists/*

COPY playwright-exporter /usr/local/bin/playwright-exporter
COPY runtime/package.json runtime/package-lock.json /opt/playwright-exporter/runtime/

# Upgrade the pinned npm CLI and install the reviewed runtime lock without
# lifecycle scripts or npx.
# Browsers are already supplied by the matching digest-pinned official image.
RUN npm install --global --engine-strict --ignore-scripts --no-audit --no-fund --cache /tmp/playwright-exporter-build-cache "npm@${NPM_VERSION}" \
    && cd /opt/playwright-exporter/runtime \
    && npm ci --engine-strict --ignore-scripts --no-audit --no-fund --cache /tmp/playwright-exporter-build-cache \
    && node -e "if (require('./node_modules/playwright/package.json').version !== '1.64.1') throw new Error('Update the base image and runtime lock together')" \
    && rm -rf /tmp/playwright-exporter-build-cache \
    && mkdir -p /opt/playwright-tests /etc/playwright-exporter \
    && ln -s /ms-playwright /opt/playwright-browsers \
    && groupadd --system playwright-exporter \
    && useradd --system --no-create-home --shell /usr/sbin/nologin -g playwright-exporter playwright-exporter \
    && chown playwright-exporter:playwright-exporter /opt/playwright-tests

# Preserve the existing browser path as an alias to the image's browser cache.
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers

COPY config.example.yaml /etc/playwright-exporter/config.yaml

EXPOSE 9115
USER playwright-exporter
# A normal non-root init reaps adopted children and forwards signals to Go.
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/playwright-exporter"]
CMD ["-config", "/etc/playwright-exporter/config.yaml"]
