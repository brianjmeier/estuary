#!/bin/bash
set -euo pipefail

REPO="brianjmeier/estuary"
INSTALL_DIR="${ESTUARY_INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="estuary"
CMD_NAME="${ESTUARY_CMD_NAME:-estuary}"

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin) OS_LABEL="darwin" ;;
  Linux) OS_LABEL="linux" ;;
  *)
    echo "Error: Unsupported OS: $OS (supported targets: macOS arm64, Linux x64)"
    exit 1
    ;;
esac

case "$ARCH" in
  arm64|aarch64) ARCH_LABEL="arm64" ;;
  x86_64) ARCH_LABEL="x64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH (supported targets: macOS arm64, Linux x64)"
    exit 1
    ;;
esac

if [ "$OS_LABEL" = "linux" ] && [ "$ARCH_LABEL" != "x64" ]; then
  echo "Error: Unsupported Linux architecture: $ARCH (supported target: Linux x64)"
  exit 1
fi

if [ "$OS_LABEL" = "darwin" ] && [ "$ARCH_LABEL" != "arm64" ]; then
  echo "Error: Unsupported macOS architecture: $ARCH (supported target: macOS arm64)"
  exit 1
fi

ASSET_NAME="${BINARY_NAME}-${OS_LABEL}-${ARCH_LABEL}"

echo "Fetching latest release..."
LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

if [ -z "$LATEST_TAG" ]; then
  echo "Error: Could not determine latest release"
  exit 1
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${ASSET_NAME}"
TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

echo "Downloading ${BINARY_NAME} ${LATEST_TAG}..."
curl -fsSL -o "$TMPFILE" "$DOWNLOAD_URL"
chmod +x "$TMPFILE"

echo "Installing to ${INSTALL_DIR}/${CMD_NAME}..."
if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "$TMPFILE" "${INSTALL_DIR}/${CMD_NAME}"
else
  sudo install -m 755 "$TMPFILE" "${INSTALL_DIR}/${CMD_NAME}"
fi

echo "Installed ${CMD_NAME} ${LATEST_TAG} to ${INSTALL_DIR}/${CMD_NAME}"
echo "Run '${CMD_NAME}' to start."
