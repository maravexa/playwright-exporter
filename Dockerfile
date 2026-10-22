# The digest fixes Node, browser revisions and OS libraries for this runtime.
FROM mcr.microsoft.com/playwright:v1.64.1-noble@sha256:7852dc5a7d72f5fe458a597d690f6c42fbc8e9e26ddfa95f8e6ef552558d9bbf

COPY playwright-exporter /usr/local/bin/playwright-exporter
COPY runtime/package.json runtime/package-lock.json /opt/playwright-exporter/runtime/

# Install only the reviewed lockfile; do not run lifecycle scripts or npx.
# Browsers are already supplied by the matching digest-pinned official image.
RUN cd /opt/playwright-exporter/runtime \
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
ENTRYPOINT ["playwright-exporter"]
CMD ["-config", "/etc/playwright-exporter/config.yaml"]
