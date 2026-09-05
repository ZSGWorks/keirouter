#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PLUGIN_DIR="$ROOT/@keirouter-opencode-plugin"
OPENCODE_CONFIG_DIR=${OPENCODE_CONFIG_DIR:-"$HOME/.config/opencode"}
TARGET_DIR="$OPENCODE_CONFIG_DIR/plugins"
TARGET="$TARGET_DIR/keirouter-plugin.js"

(cd "$PLUGIN_DIR" && npm run build)
mkdir -p "$TARGET_DIR"
cp "$PLUGIN_DIR/dist/index.js" "$TARGET"

printf 'Installed KeiRouter OpenCode plugin: %s\n' "$TARGET"
