#!/bin/bash
# install.sh - Install plccli on macOS

# Detect architecture
if [[ $(uname -m) == 'arm64' ]]; then
  ARCH="arm64"
else
  ARCH="amd64"
fi

echo "Installing plccli ($ARCH)..."

# Download and install
curl -L "https://github.com/o16s/plccli/releases/latest/download/plccli-darwin-$ARCH.tar.gz" -o plccli.tar.gz && \
tar -xzf plccli.tar.gz && \
sudo mv "plccli-darwin-$ARCH" /usr/local/bin/plccli && \
sudo chmod +x /usr/local/bin/plccli && \
rm plccli.tar.gz

# Verify installation
echo "Installation complete! Verifying..."
plccli --version