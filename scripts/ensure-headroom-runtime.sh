#!/usr/bin/env bash
set -euo pipefail

# Provision the private native Headroom runtime used by `keirouter start`.
# It does not start a proxy; the Go process owns that lifecycle.

UV_VERSION="0.12.0"
DATA_DIR="${KEIROUTER_DATA__DIR:-${KEIROUTER_DATA_DIR:-$HOME/.keirouter}}"
RUNTIME_DIR="${KEIROUTER_HEADROOM_RUNTIME_DIR:-$DATA_DIR/headroom-runtime}"
TOOLS_DIR="$RUNTIME_DIR/tools"
BOOTSTRAP_DIR="$RUNTIME_DIR/bootstrap"
UV_BIN="$TOOLS_DIR/uv"
VENV_DIR="$RUNTIME_DIR/venv"
PYTHON_DIR="$RUNTIME_DIR/python"
CACHE_DIR="$RUNTIME_DIR/cache"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REQUIREMENTS="$SCRIPT_DIR/headroom-requirements.txt"

die() { printf 'Headroom runtime setup failed: %s\n' "$*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

case "$(uname -s)" in
  Darwin|Linux) ;;
  *) die "native Headroom provisioning supports macOS and Linux; use Docker on this platform" ;;
esac

need_cmd curl
need_cmd sh
need_cmd tar
mkdir -p "$TOOLS_DIR" "$BOOTSTRAP_DIR"

# Keep a self-contained retry entrypoint beside the private runtime. This lets
# `keirouter start` retry provisioning after an interrupted setup without
# consulting PATH or the user's shell configuration.
if [ "$SCRIPT_DIR" != "$BOOTSTRAP_DIR" ]; then
  cp "$SCRIPT_DIR/ensure-headroom-runtime.sh" "$BOOTSTRAP_DIR/ensure-headroom-runtime.sh"
  cp "$REQUIREMENTS" "$BOOTSTRAP_DIR/headroom-requirements.txt"
  chmod 700 "$BOOTSTRAP_DIR/ensure-headroom-runtime.sh"
fi

uv_archive() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os:$arch" in
    Darwin:arm64|Darwin:aarch64)
      UV_ARCHIVE="uv-aarch64-apple-darwin.tar.gz"
      UV_SHA256="2b9e582af54f84fa50c115427451a6c13e80f43b52f8282b8af5791077317bbf"
      ;;
    Darwin:x86_64)
      UV_ARCHIVE="uv-x86_64-apple-darwin.tar.gz"
      UV_SHA256="d41593beaefc54bab7d062af0ef6ca093bfb81d001d58ebbef39e44423f9c496"
      ;;
    Linux:aarch64|Linux:arm64)
      if ldd --version 2>&1 | grep -qi musl; then
        UV_ARCHIVE="uv-aarch64-unknown-linux-musl.tar.gz"
        UV_SHA256="936fbbf20188a2b1c66bce3dca3f4009a5c9cdf12bb2bbd084e71926f75d6a15"
      else
        UV_ARCHIVE="uv-aarch64-unknown-linux-gnu.tar.gz"
        UV_SHA256="2c5d6e3092cc5223b10ff403880cc75121bf64e84644e7a0c69f643b0d89ac95"
      fi
      ;;
    Linux:x86_64)
      if ldd --version 2>&1 | grep -qi musl; then
        UV_ARCHIVE="uv-x86_64-unknown-linux-musl.tar.gz"
        UV_SHA256="3340a9d8cffc4d801bc1a7459ebfaf5790c79400720d9b6963d806f058526684"
      else
        UV_ARCHIVE="uv-x86_64-unknown-linux-gnu.tar.gz"
        UV_SHA256="eaf842262aa1c418d8ecc5605f02ee1ebfd369124fa48548e85f9481a47831a9"
      fi
      ;;
    *) die "unsupported native platform $os/$arch" ;;
  esac
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

if [ ! -x "$UV_BIN" ]; then
  uv_archive
  archive_path="$TOOLS_DIR/$UV_ARCHIVE"
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
    "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/${UV_ARCHIVE}" \
    --output "$archive_path"
  [ "$(sha256 "$archive_path")" = "$UV_SHA256" ] || die "uv archive checksum mismatch"
  tar -xzf "$archive_path" --strip-components=1 -C "$TOOLS_DIR"
  rm -f "$archive_path"
fi

uv() {
  env UV_PYTHON_INSTALL_DIR="$PYTHON_DIR" UV_CACHE_DIR="$CACHE_DIR" "$UV_BIN" "$@"
}

PYTHON_BIN=""
for candidate in "$PYTHON_DIR"/*/bin/python3; do
  if [ -x "$candidate" ]; then
    PYTHON_BIN="$candidate"
    break
  fi
done
if [ -z "$PYTHON_BIN" ]; then
  uv python install --no-bin --install-dir "$PYTHON_DIR" 3.13
  for candidate in "$PYTHON_DIR"/*/bin/python3; do
    if [ -x "$candidate" ]; then
      PYTHON_BIN="$candidate"
      break
    fi
  done
fi
[ -n "$PYTHON_BIN" ] || die "uv did not install Python 3.13 under $PYTHON_DIR"

uv venv --python "$PYTHON_BIN" "$VENV_DIR"
uv pip install --python "$VENV_DIR/bin/python" --require-hashes -r "$REQUIREMENTS"

printf 'Headroom runtime prepared at %s\n' "$VENV_DIR"
