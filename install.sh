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

# Check if the extracted binary exists
if [ ! -f "plccli-$OS_NAME-$ARCH" ]; then
  echo "Error: Expected binary 'plccli-$OS_NAME-$ARCH' not found after extraction"
  rm -f plccli.tar.gz
  exit 1
fi

# Installation functions
install_system_wide_with_sudo() {
  echo "Installing system-wide to /usr/local/bin/plccli (with sudo)"
  if sudo mkdir -p /usr/local/bin && \
     sudo mv "plccli-$OS_NAME-$ARCH" /usr/local/bin/plccli && \
     sudo chmod +x /usr/local/bin/plccli; then
    INSTALL_PATH="/usr/local/bin/plccli"
    return 0
  else
    return 1
  fi
}

install_system_wide_direct() {
  echo "Installing system-wide to /usr/local/bin/plccli (direct)"
  if mkdir -p /usr/local/bin 2>/dev/null && \
     mv "plccli-$OS_NAME-$ARCH" /usr/local/bin/plccli 2>/dev/null && \
     chmod +x /usr/local/bin/plccli 2>/dev/null; then
    INSTALL_PATH="/usr/local/bin/plccli"
    return 0
  else
    return 1
  fi
}

install_user_local() {
  echo "Installing to user directory ~/.local/bin/plccli"
  if mkdir -p "$HOME/.local/bin" && \
     mv "plccli-$OS_NAME-$ARCH" "$HOME/.local/bin/plccli" && \
     chmod +x "$HOME/.local/bin/plccli"; then
    INSTALL_PATH="$HOME/.local/bin/plccli"
    
    # Add to PATH if needed
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
      echo "Adding ~/.local/bin to your PATH"
      if [[ -f "$HOME/.bashrc" ]]; then
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"
        echo "Please run 'source ~/.bashrc' or start a new terminal to update your PATH"
      elif [[ -f "$HOME/.zshrc" ]]; then
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.zshrc"
        echo "Please run 'source ~/.zshrc' or start a new terminal to update your PATH"
      else
        echo "Please add $HOME/.local/bin to your PATH manually"
      fi
    fi
    return 0
  else
    return 1
  fi
}

install_current_dir() {
  echo "Installing to current directory (./plccli)"
  if mv "plccli-$OS_NAME-$ARCH" "./plccli" && \
     chmod +x "./plccli"; then
    INSTALL_PATH="$(pwd)/plccli"
    return 0
  else
    return 1
  fi
}

# Try installation methods in order of preference
INSTALL_SUCCESS=false

# Method 1: Try system-wide with sudo (if sudo exists)
if command -v sudo >/dev/null 2>&1; then
  if install_system_wide_with_sudo; then
    INSTALL_SUCCESS=true
  fi
fi

# Method 2: Try system-wide without sudo (if we have write permissions)
if [ "$INSTALL_SUCCESS" = false ]; then
  if install_system_wide_direct; then
    INSTALL_SUCCESS=true
  fi
fi

# Method 3: Try user local directory
if [ "$INSTALL_SUCCESS" = false ]; then
  if install_user_local; then
    INSTALL_SUCCESS=true
  fi
fi

# Method 4: Fall back to current directory
if [ "$INSTALL_SUCCESS" = false ]; then
  if install_current_dir; then
    INSTALL_SUCCESS=true
  fi
fi

# Check if any installation method succeeded
if [ "$INSTALL_SUCCESS" = false ]; then
  echo "Error: All installation methods failed. Please install manually."
  rm -f plccli.tar.gz
  exit 1
fi

# Clean up
rm -f plccli.tar.gz

# Verify installation
echo -e "\nInstallation complete!"
echo "Binary installed to: $INSTALL_PATH"
echo -e "\nVerifying installation..."

if command -v plccli >/dev/null 2>&1; then
  plccli --version
  echo -e "\nSuccess! You can now use plccli from anywhere."
elif [ -f "$INSTALL_PATH" ]; then
  "$INSTALL_PATH" --version
  echo -e "\nSuccess! Binary is installed at: $INSTALL_PATH"
  if [[ "$INSTALL_PATH" != *"/usr/local/bin/"* ]] && [[ "$INSTALL_PATH" != *"/.local/bin/"* ]]; then
    echo "Note: plccli is not in your PATH. You can run it directly using: $INSTALL_PATH"
  fi
else
  echo "Warning: Installation may have failed. Binary not found at expected location."
fi

echo -e "\nExample usage:"
echo "  $INSTALL_PATH --service --endpoint opc.tcp://your-plc-ip:4840 --username \"username\" --password \"password\""
echo "  $INSTALL_PATH opcua get ns=3;s=MyVariable"