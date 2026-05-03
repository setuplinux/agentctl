#!/usr/bin/env bash
set -euo pipefail

repo="${AGENTCTL_REPO:-setuplinux/agentctl}"
install_dir="${AGENTCTL_INSTALL_DIR:-$HOME/.local/bin}"
version="${AGENTCTL_VERSION:-latest}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

asset="agentctl-${os}-${arch}"
base="https://github.com/${repo}/releases"
if [ "$version" = "latest" ]; then
  url="${base}/latest/download/${asset}"
else
  url="${base}/download/${version}/${asset}"
fi

mkdir -p "$install_dir"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "Downloading ${asset} from ${repo}..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$url" -o "$tmp"
elif command -v wget >/dev/null 2>&1; then
  wget -q "$url" -O "$tmp"
else
  echo "Need curl or wget to install agentctl." >&2
  exit 1
fi

chmod +x "$tmp"
mv "$tmp" "${install_dir}/agentctl"

echo "Installed agentctl to ${install_dir}/agentctl"
if ! command -v agentctl >/dev/null 2>&1; then
  echo "Note: ${install_dir} is not on PATH for this shell."
  echo "Add this to your shell profile: export PATH=\"${install_dir}:\$PATH\""
fi
"${install_dir}/agentctl" status || true
