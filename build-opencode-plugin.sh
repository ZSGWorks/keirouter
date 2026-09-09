#!/usr/bin/env sh
set -eu

# The installer intentionally talks only to a local KeiRouter instance. It
# creates an inbound gateway key once, then leaves OpenCode to use the normal
# plugin authentication and dynamic model-discovery paths.

umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PLUGIN_DIR="$ROOT/@keirouter-opencode-plugin"
OPENCODE_CONFIG_DIR=${OPENCODE_CONFIG_DIR:-"${XDG_CONFIG_HOME:-$HOME/.config}/opencode"}
OPENCODE_DATA_DIR=${OPENCODE_DATA_DIR:-"${XDG_DATA_HOME:-$HOME/.local/share}/opencode"}
TARGET_DIR="$OPENCODE_CONFIG_DIR/plugins"
TARGET="$TARGET_DIR/keirouter-plugin.js"
AUTH_FILE="$OPENCODE_DATA_DIR/auth.json"
KEIROUTER_URL=${KEIROUTER_URL:-"http://127.0.0.1:${KEIROUTER_PORT:-20180}"}
COOKIE_JAR=""
AUTH_CURL_CONFIG=""
TTY_ECHO_DISABLED=false
DASHBOARD_PASSWORD_SET=false
DASHBOARD_PASSWORD=""

if [ "${KEIROUTER_DASHBOARD_PASSWORD+x}" = x ]; then
  DASHBOARD_PASSWORD_SET=true
  DASHBOARD_PASSWORD=$KEIROUTER_DASHBOARD_PASSWORD
  unset KEIROUTER_DASHBOARD_PASSWORD
fi

info() { printf '> %s\n' "$*"; }
ok() { printf 'OK %s\n' "$*"; }
die() { printf 'ERROR %s\n' "$*" >&2; exit 1; }

cleanup() {
  if [ "$TTY_ECHO_DISABLED" = true ]; then
    stty echo </dev/tty 2>/dev/null || true
    printf '\n' >&2
  fi
  [ -z "$COOKIE_JAR" ] || rm -f "$COOKIE_JAR"
  [ -z "$AUTH_CURL_CONFIG" ] || rm -f "$AUTH_CURL_CONFIG"
}
trap cleanup EXIT HUP INT TERM

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found"
}

normalize_local_url() {
  node -e '
const raw = process.argv[1];
let url;
try {
  url = new URL(raw);
} catch {
  process.exit(1);
}
const localHosts = new Set(["127.0.0.1", "localhost", "::1"]);
if (
  url.protocol !== "http:" ||
  !localHosts.has(url.hostname) ||
  url.username || url.password || url.search || url.hash ||
  (url.pathname !== "" && url.pathname !== "/")
) {
  process.exit(1);
}
process.stdout.write(url.origin);
' "$1"
}

read_saved_key() {
  node -e '
const fs = require("fs");
const file = process.argv[1];
if (!fs.existsSync(file)) process.exit(0);
let auth;
try {
  auth = JSON.parse(fs.readFileSync(file, "utf8"));
} catch {
  process.exit(2);
}
if (!auth || typeof auth !== "object" || Array.isArray(auth)) process.exit(2);
const entry = auth.keirouter;
if (entry && typeof entry === "object" && entry.type === "api" && typeof entry.key === "string" && entry.key) {
  process.stdout.write(entry.key);
}
' "$AUTH_FILE"
}

write_auth_header() {
  key="$1"
  AUTH_CURL_CONFIG=$(mktemp "${TMPDIR:-/tmp}/keirouter-opencode-curl.XXXXXX")
  chmod 600 "$AUTH_CURL_CONFIG"
  printf 'header = "Authorization: Bearer %s"\n' "$key" >"$AUTH_CURL_CONFIG"
}

verify_key() {
  key="$1"
  write_auth_header "$key"
  if curl --config "$AUTH_CURL_CONFIG" -fsS "$KEIROUTER_URL/v1/models" >/dev/null; then
    rm -f "$AUTH_CURL_CONFIG"
    AUTH_CURL_CONFIG=""
    return 0
  fi
  rm -f "$AUTH_CURL_CONFIG"
  AUTH_CURL_CONFIG=""
  return 1
}

read_dashboard_password() {
  if [ "$DASHBOARD_PASSWORD_SET" = true ]; then
    [ -n "$DASHBOARD_PASSWORD" ] || die "KEIROUTER_DASHBOARD_PASSWORD must not be empty"
    printf '%s' "$DASHBOARD_PASSWORD"
    return
  fi

  [ -r /dev/tty ] || die "set KEIROUTER_DASHBOARD_PASSWORD when no terminal is available"
  printf 'KeiRouter dashboard password: ' >&2
  stty -echo </dev/tty
  TTY_ECHO_DISABLED=true
  IFS= read -r password </dev/tty || true
  stty echo </dev/tty
  TTY_ECHO_DISABLED=false
  printf '\n' >&2
  [ -n "$password" ] || die "dashboard password must not be empty"
  printf '%s' "$password"
}

login_and_create_key() {
  password=$(read_dashboard_password)
  COOKIE_JAR=$(mktemp "${TMPDIR:-/tmp}/keirouter-opencode-cookie.XXXXXX")
  chmod 600 "$COOKIE_JAR"

  if ! login_response=$(printf '%s' "$password" | node -e '
const chunks = [];
process.stdin.on("data", (chunk) => chunks.push(chunk));
process.stdin.on("end", () => process.stdout.write(JSON.stringify({ password: Buffer.concat(chunks).toString("utf8") })));
' | curl -fsS -c "$COOKIE_JAR" -H 'Content-Type: application/json' --data-binary @- "$KEIROUTER_URL/api/auth/login"); then
    rm -f "$COOKIE_JAR"
    COOKIE_JAR=""
    return 1
  fi
  if ! printf '%s' "$login_response" | node -e '
const chunks = [];
process.stdin.on("data", (chunk) => chunks.push(chunk));
process.stdin.on("end", () => {
  try {
    const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    if (!body || body.ok !== true) process.exit(1);
  } catch {
    process.exit(1);
  }
});
'; then
    rm -f "$COOKIE_JAR"
    COOKIE_JAR=""
    return 1
  fi

  if ! key_response=$(printf '%s' '{"name":"OpenCode"}' | curl -fsS -b "$COOKIE_JAR" -H 'Content-Type: application/json' --data-binary @- "$KEIROUTER_URL/api/keys"); then
    rm -f "$COOKIE_JAR"
    COOKIE_JAR=""
    return 1
  fi
  if ! api_key=$(printf '%s' "$key_response" | node -e '
const chunks = [];
process.stdin.on("data", (chunk) => chunks.push(chunk));
process.stdin.on("end", () => {
  try {
    const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    if (!body || typeof body.key !== "string" || !body.key.startsWith("kr_")) process.exit(1);
    process.stdout.write(body.key);
  } catch {
    process.exit(1);
  }
});
' ); then
    rm -f "$COOKIE_JAR"
    COOKIE_JAR=""
    return 1
  fi
  rm -f "$COOKIE_JAR"
  COOKIE_JAR=""
  printf '%s' "$api_key"
}

save_key() {
  key="$1"
  printf '%s' "$key" | node -e '
const fs = require("fs");
const path = require("path");
const file = process.argv[1];
const key = fs.readFileSync(0, "utf8");
if (!key.startsWith("kr_")) process.exit(1);
let auth = {};
if (fs.existsSync(file)) {
  auth = JSON.parse(fs.readFileSync(file, "utf8"));
  if (!auth || typeof auth !== "object" || Array.isArray(auth)) process.exit(1);
}
auth.keirouter = { type: "api", key };
fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
const temp = path.join(path.dirname(file), `.${path.basename(file)}.${process.pid}.tmp`);
fs.writeFileSync(temp, `${JSON.stringify(auth, null, 2)}\n`, { mode: 0o600 });
fs.chmodSync(temp, 0o600);
fs.renameSync(temp, file);
' "$AUTH_FILE" || die "could not save the OpenCode credential"
}

need_cmd curl
need_cmd node
need_cmd npm
KEIROUTER_URL=$(normalize_local_url "$KEIROUTER_URL") || die "KEIROUTER_URL must be a loopback HTTP URL without a path"

(cd "$PLUGIN_DIR" && npm run build)
mkdir -p "$TARGET_DIR"
cp "$PLUGIN_DIR/dist/index.js" "$TARGET"

curl -fsS "$KEIROUTER_URL/api/auth/status" >/dev/null || die "KeiRouter is not reachable at $KEIROUTER_URL"

if ! saved_key=$(read_saved_key); then
  die "existing OpenCode auth file is invalid; refusing to overwrite it"
fi

if [ -n "$saved_key" ] && verify_key "$saved_key"; then
  ok "KeiRouter OpenCode credential is already valid"
  printf 'Installed KeiRouter OpenCode plugin: %s\n' "$TARGET"
  exit 0
fi

info "Creating a KeiRouter API key for OpenCode"
api_key=$(login_and_create_key) || die "KeiRouter API-key creation returned an unexpected response"
verify_key "$api_key" || die "new KeiRouter API key could not discover models"
save_key "$api_key"

ok "KeiRouter OpenCode credential provisioned"
printf 'Installed KeiRouter OpenCode plugin: %s\n' "$TARGET"
