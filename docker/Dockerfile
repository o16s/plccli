FROM telegraf:latest

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    bash \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Install plccli
RUN curl -s https://raw.githubusercontent.com/o16s/plccli/main/install.sh | bash

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Copy health check script
COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

# Configure Docker's built-in health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD /healthcheck.sh

  # Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]