FROM telegraf:latest

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    bash \
    procps \
    findutils \
    && rm -rf /var/lib/apt/lists/*

# Install plccli with verbose debugging
RUN set -x \
    && curl -s https://raw.githubusercontent.com/o16s/plccli/main/install.sh > /tmp/install.sh \
    && cat /tmp/install.sh \
    && bash /tmp/install.sh \
    && echo "Finding plccli binary:" \
    && find / -name "plccli*" 2>/dev/null || echo "Not found" \
    && mkdir -p /usr/bin \
    && if [ -f /usr/local/bin/plccli ]; then \
          cp -v /usr/local/bin/plccli /usr/bin/plccli; \
       elif [ -f /root/.local/bin/plccli ]; then \
          cp -v /root/.local/bin/plccli /usr/bin/plccli; \
       elif [ -f /usr/bin/plccli ]; then \
          echo "plccli already in /usr/bin"; \
       else \
          find / -name "plccli*" 2>/dev/null | xargs -I{} cp -v {} /usr/bin/plccli || echo "Could not find plccli"; \
       fi \
    && ls -la /usr/bin/plccli || echo "Failed to create /usr/bin/plccli"

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Copy health check script
COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

# Configure Docker's built-in health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD /healthcheck.sh

# Set PATH explicitly
ENV PATH="/usr/local/bin:/usr/bin:${PATH}"

# Entrypoint that shows where plccli might be
ENTRYPOINT ["bash", "-c", "echo 'Final check:' && find / -name plccli* 2>/dev/null || echo 'Not found' && exec /entrypoint.sh"]