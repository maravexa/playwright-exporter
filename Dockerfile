FROM mcr.microsoft.com/playwright:v1.48.0-noble

# Install the exporter binary
COPY playwright-exporter /usr/local/bin/playwright-exporter

# Create directories
RUN mkdir -p /opt/playwright-tests /opt/playwright-browsers /etc/playwright-exporter \
    && groupadd --system playwright-exporter \
    && useradd --system --no-create-home --shell /usr/sbin/nologin -g playwright-exporter playwright-exporter \
    && chown -R playwright-exporter:playwright-exporter /opt/playwright-tests /opt/playwright-browsers

# Set browser path
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers

# Install only Chromium
RUN npx --yes playwright install chromium

# Copy default config
COPY config.example.yaml /etc/playwright-exporter/config.yaml

EXPOSE 9115

USER playwright-exporter

ENTRYPOINT ["playwright-exporter"]
CMD ["-config", "/etc/playwright-exporter/config.yaml"]
