#!/usr/bin/env bash
# Build the production image and deploy it to localhost against the host
# Postgres for production-grade local testing.
#
#   ./scripts/dev-deploy.sh          (or: make dev-deploy)
#
# - Port 20181 on 127.0.0.1 only; `make dev` can keep :20180.
# - Database: local Postgres on :5432, database keirouter_dev (created if
#   missing, using local-user trust auth). Override KEIROUTER_DATABASE__DSN
#   in .env for custom credentials.
# - Master key: generated once into .env and reused, so encrypted provider
#   credentials survive redeploys.
#
# Teardown: docker compose -f compose.yaml -f compose.dev-deploy.yaml down
set -euo pipefail

cd "$(dirname "$0")/.."

# Port priority: env var -> .env -> 20181 (mirrors how MASTER_KEY/DSN are read).
PORT="${KEIROUTER_PORT:-$(grep -E '^KEIROUTER_PORT=' .env 2>/dev/null | tail -1 | cut -d= -f2- || true)}"
PORT="${PORT:-20181}"
DB_NAME="keirouter_dev"
COMPOSE_FILES=(-f compose.yaml -f compose.dev-deploy.yaml)
HEALTH_URL="http://127.0.0.1:${PORT}/healthz"

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33mwarn:\033[0m %s\n' "$*"; }
die()  { printf '\033[0;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# ---- preflight -------------------------------------------------------------
command -v docker >/dev/null 2>&1 || die "docker not found"
docker compose version >/dev/null 2>&1 || die "docker compose v2 required"
# !override port merge needs Compose v2.24+.
COMPOSE_VERSION="$(docker compose version --short 2>/dev/null || echo 0)"
if [ "$(printf '%s\n2.24' "$COMPOSE_VERSION" | sort -V | head -1)" != "2.24" ]; then
  die "docker compose >= 2.24 required (found ${COMPOSE_VERSION}); needed for ports: !override"
fi

# ---- ensure .env -----------------------------------------------------------
if [ ! -f .env ]; then
  log "Creating .env"
  cat > .env <<EOF
KEIROUTER_PORT=${PORT}
KEIROUTER_MASTER_KEY=
EOF
fi

# Read master key from .env (last assignment wins, same as compose).
MASTER_KEY="$(grep -E '^KEIROUTER_MASTER_KEY=' .env | tail -1 | cut -d= -f2- || true)"
if [ -z "$MASTER_KEY" ]; then
  log "Generating KEIROUTER_MASTER_KEY into .env"
  MASTER_KEY="$(openssl rand -base64 32)"
  # Remove existing (empty) line, then append the generated key.
  grep -vE '^KEIROUTER_MASTER_KEY=' .env > .env.tmp || true
  printf 'KEIROUTER_MASTER_KEY=%s\n' "$MASTER_KEY" >> .env.tmp
  mv .env.tmp .env
fi
export KEIROUTER_MASTER_KEY="$MASTER_KEY"
export KEIROUTER_PORT="$PORT"

# ---- resolve Postgres DSN --------------------------------------------------
# Default: local user over trust auth against the host Postgres. The resolved
# DSN is persisted to .env so ad-hoc compose commands (logs/down) interpolate
# the same value. An explicit KEIROUTER_DATABASE__DSN in .env always wins.
DSN_FROM_ENV="$(grep -E '^KEIROUTER_DATABASE__DSN=' .env | tail -1 | cut -d= -f2- || true)"
if [ -n "$DSN_FROM_ENV" ]; then
  DSN="$DSN_FROM_ENV"
  log "Using KEIROUTER_DATABASE__DSN from .env"
else
  DSN="postgres://$(id -un)@host.docker.internal:5432/${DB_NAME}?sslmode=disable"
  printf 'KEIROUTER_DATABASE__DSN=%s\n' "$DSN" >> .env
fi
export KEIROUTER_DATABASE__DSN="$DSN"

# ---- ensure local database -------------------------------------------------
# Reachability is probed whenever the DSN targets the host Postgres (the
# auto-generated default, or a custom DSN that still uses host.docker.internal).
# Other custom DSNs opt out — the container healthcheck surfaces their
# connection failures, matching the pre-hardening behavior.
dsn_uses_host_pg() {
  case "$DSN" in
    *host.docker.internal*) return 0 ;;
    *) return 1 ;;
  esac
}

if dsn_uses_host_pg; then
  if ! command -v psql >/dev/null 2>&1; then
    die "psql not found but KEIROUTER_DATABASE__DSN points at host Postgres; install postgresql client tools (e.g. brew install libpq) or set KEIROUTER_DATABASE__DSN in .env to a reachable database"
  fi

  # Fail fast instead of warning + 60s healthcheck timeout. Capture stderr so
  # we can recognize the stale postmaster.pid symptom and hint at the fix.
  if ! PG_PROBE_ERR="$(psql -d postgres -tAc "SELECT 1" 2>&1)"; then
    if printf '%s' "$PG_PROBE_ERR" | grep -q 'lock file "postmaster.pid" already exists'; then
      PGDATA_DIR="$(brew --prefix 2>/dev/null || echo /opt/homebrew)/var/postgresql@18"
      die "host Postgres has a stale postmaster.pid lock:
$PG_PROBE_ERR

A previous postgres shutdown did not clean up its lock. Remove it and restart:
    rm \"$PGDATA_DIR/postmaster.pid\"
    brew services restart postgresql@18"
    fi
    die "host Postgres not reachable: $PG_PROBE_ERR

Hint: start it with 'brew services start postgresql@18' (or set KEIROUTER_DATABASE__DSN in .env to point elsewhere)"
  fi
fi

if [ -z "$DSN_FROM_ENV" ]; then
  if ! psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" 2>/dev/null | grep -q 1; then
    log "Creating database ${DB_NAME}"
    createdb "$DB_NAME" || die "could not create ${DB_NAME} (createdb failed)"
  else
    log "Database ${DB_NAME} exists"
  fi
fi

# ---- build + deploy --------------------------------------------------------
log "Building image and starting container on 127.0.0.1:${PORT}"
docker compose "${COMPOSE_FILES[@]}" up -d --build

# ---- wait for health -------------------------------------------------------
log "Waiting for ${HEALTH_URL}"
ready=0
for _ in $(seq 1 60); do
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  docker compose "${COMPOSE_FILES[@]}" logs --tail 50 keirouter
  die "container did not become healthy within 60s (see logs above; check DSN/Postgres auth)"
fi

log "Deployed: dashboard http://127.0.0.1:${PORT} (default password: keirouter — change on first login)"
log "Logs:     docker compose ${COMPOSE_FILES[*]} logs -f keirouter"
log "Teardown: docker compose ${COMPOSE_FILES[*]} down"