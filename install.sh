#!/bin/sh
# rute installer — curl -fsSL https://raw.githubusercontent.com/Sandbye/rute/main/install.sh | sh
set -e

REPO="Sandbye/rute"
INSTALL_DIR="/usr/local/bin"

# Detect OS and arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest version
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"

if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version"
  exit 1
fi

FILENAME="rute_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"

echo "Installing rute v${VERSION} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${FILENAME}"
tar -xzf "${TMP}/${FILENAME}" -C "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/rute" "${INSTALL_DIR}/rute"
else
  sudo mv "${TMP}/rute" "${INSTALL_DIR}/rute"
fi

echo "rute v${VERSION} installed to ${INSTALL_DIR}/rute"
