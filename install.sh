#!/bin/bash
# install.sh - Install plccli on Linux or macOS

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [[ "$OS" == "darwin" ]]; then
  OS_NAME="darwin"
elif [[ "$OS" == "linux" ]]; then
  OS_NAME="linux"
else
  echo "Unsupported operating system: $OS"
  exit 1
fi

# Detect architecture
if [[ $(uname -m) == 'arm64' ]]; then
  ARCH="arm64"
else
  ARCH="amd64"
fi

echo "Installing plccli for $OS_NAME ($ARCH)..."

# Download the binary
DOWNLOAD_URL="https://github.com/o16s/plccli/releases/latest/download/plccli-$OS_NAME-$ARCH.tar.gz"
echo "Downloading from $DOWNLOAD_URL"
curl -L "$DOWNLOAD_URL" -o plccli.tar.gz
if [ $? -ne 0 ]; then
  echo "Download failed. Please check your internet connection and try again."
  rm -f plccli.tar.gz
  exit 1
fi

# Extract the binary
tar -xzf plccli.tar.gz
if [ $? -ne 0 ]; then
  echo "Extraction failed. The downloaded file may be corrupted."
  rm -f plccli.tar.gz
  exit 1
fi

# Determine the best installation path
install_system_wide() {
  echo "Installing system-wide to /usr/local/bin/plccli"
  sudo mkdir -p /usr/local/bin
  sudo mv "plccli-$OS_NAME-$ARCH" /usr/local/bin/plccli
  sudo chmod +x /usr/local/bin/plccli
  INSTALL_PATH="/usr/local/bin/plccli"
  return $?
}

install_user_local() {
  echo "Installing to user directory ~/.local/bin/plccli"
  mkdir -p $HOME/.local/bin
  mv "plccli-$OS_NAME-$ARCH" $HOME/.local/bin/plccli
  chmod +x $HOME/.local/bin/plccli
  INSTALL_PATH="$HOME/.local/bin/plccli"
  
  # Add to PATH if needed
  if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    echo "Adding ~/.local/bin to your PATH"
    if [[ -f "$HOME/.bashrc" ]]; then
      echo 'export PATH="$HOME/.local/bin:$PATH"' >> $HOME/.bashrc
      echo "Please run 'source ~/.bashrc' or start a new terminal to update your PATH"
    elif [[ -f "$HOME/.zshrc" ]]; then
      echo 'export PATH="$HOME/.local/bin:$PATH"' >> $HOME/.zshrc
      echo "Please run 'source ~/.zshrc' or start a new terminal to update your PATH"
    else
      echo "Please add $HOME/.local/bin to your PATH manually"
    fi
  fi
  return 0
}

# Try system-wide installation first, fall back to user directory
install_system_wide || install_user_local

# Clean up
rm -f plccli.tar.gz

# Verify installation
echo -e "\nInstallation complete!"
echo "Binary installed to: $INSTALL_PATH"
echo -e "\nVerifying installation..."

if command -v plccli >/dev/null 2>&1; then
  plccli --version
  echo -e "\nSuccess! You can now use plccli from anywhere."
else
  echo "plccli is installed but not immediately available in your PATH."
  echo "You can run it directly using: $INSTALL_PATH"
  echo "Or restart your terminal session to update your PATH."
fi

echo -e "\nExample usage:"
echo "  $INSTALL_PATH --service --endpoint opc.tcp://your-plc-ip:4840 --username \"username\" --password \"password\""
echo "  $INSTALL_PATH opcua get ns=3;s=MyVariable"