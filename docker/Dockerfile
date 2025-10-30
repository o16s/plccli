FROM telegraf:latest

# Install dependencies including networking tools
RUN apt-get update && apt-get install -y \
    curl \
    bash \
    procps \
    findutils \
    iputils-ping \
    iproute2 \
    net-tools \
    && rm -rf /var/lib/apt/lists/*

# Install plccli for the correct architecture
RUN set -x \
    && ARCH=$(uname -m) \
    && if [ "$ARCH" = "aarch64" ]; then PLCCLI_ARCH="arm64"; elif [ "$ARCH" = "x86_64" ]; then PLCCLI_ARCH="amd64"; else PLCCLI_ARCH="amd64"; fi \
    && echo "Detected architecture: $ARCH, using plccli arch: $PLCCLI_ARCH" \
    && curl -L "https://github.com/o16s/plccli/releases/latest/download/plccli-linux-${PLCCLI_ARCH}.tar.gz" -o plccli.tar.gz \
    && tar -xzf plccli.tar.gz \
    && mv "plccli-linux-${PLCCLI_ARCH}" /usr/bin/plccli \
    && chmod +x /usr/bin/plccli \
    && rm plccli.tar.gz \
    && echo "plccli installed successfully" \
    && /usr/bin/plccli --version

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Copy health check script
COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

# Configure Docker's built-in health check
HEALTHCHECK --interval=60s --timeout=10s --start-period=60s --retries=3 \
  CMD /healthcheck.sh

# Set PATH explicitly
ENV PATH="/usr/local/bin:/usr/bin:${PATH}"

# Normal entrypoint
ENTRYPOINT ["/entrypoint.sh"]
CMD ["telegraf"]