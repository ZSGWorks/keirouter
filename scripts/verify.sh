#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
POSTGRES_NAME="keirouter-verify-postgres-$$"
POSTGRES_STARTED=false
COMPOSE_PROJECT="keirouter-verify-headroom-$$"
COMPOSE_STARTED=false
COMPOSE_COOKIE=""

cleanup() {
  if [[ "$COMPOSE_STARTED" == true ]]; then
    docker compose --project-name "$COMPOSE_PROJECT" -f "$REPO_ROOT/compose.yaml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  if [[ -n "$COMPOSE_COOKIE" ]]; then
    rm -f "$COMPOSE_COOKIE"
  fi
  if [[ "$POSTGRES_STARTED" == true ]]; then
    docker rm --force "$POSTGRES_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

run() {
  printf '\n==> %s\n' "$1"
  shift
  "$@"
}

wait_for_postgres() {
  local status

  for _ in $(seq 1 30); do
    status="$(docker inspect --format '{{.State.Health.Status}}' "$POSTGRES_NAME")"
    if [[ "$status" == "healthy" ]]; then
      return
    fi
    if [[ "$status" == "unhealthy" ]]; then
      docker logs "$POSTGRES_NAME" >&2
      return 1
    fi
    sleep 1
  done

  printf 'PostgreSQL did not become healthy.\n' >&2
  docker logs "$POSTGRES_NAME" >&2
  return 1
}

wait_for_compose_service() {
  local service="$1"
  local container status

  for _ in $(seq 1 60); do
    container="$(docker compose --project-name "$COMPOSE_PROJECT" -f compose.yaml ps -q "$service")"
    if [[ -n "$container" ]]; then
      status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container")"
      if [[ "$status" == "healthy" ]]; then
        return
      fi
      if [[ "$status" == "unhealthy" || "$status" == "exited" ]]; then
        docker compose --project-name "$COMPOSE_PROJECT" -f compose.yaml logs "$service" >&2
        return 1
      fi
    fi
    sleep 1
  done

  printf 'Compose service %s did not become healthy.\n' "$service" >&2
  docker compose --project-name "$COMPOSE_PROJECT" -f compose.yaml logs "$service" >&2
  return 1
}

cd "$REPO_ROOT"

require_command go
require_command npm
require_command docker
require_command curl

if ! docker info >/dev/null 2>&1; then
  printf 'Docker daemon is unavailable. Start Docker and retry.\n' >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  printf 'Docker Compose v2 is unavailable. Install it and retry.\n' >&2
  exit 1
fi

run 'Download Go modules' bash -c 'cd backend && go mod download'
run 'Vet backend' make vet
run 'Test backend' make test

run 'Install frontend dependencies' bash -c 'cd frontend && npm ci'
run 'Typecheck frontend' bash -c 'cd frontend && npm run typecheck'
run 'Build frontend' bash -c 'cd frontend && npm run build'

run 'Start temporary PostgreSQL'
docker run --detach --rm --name "$POSTGRES_NAME" \
  --health-cmd 'pg_isready -U postgres -d keirouter_test' \
  --health-interval 1s \
  --health-timeout 5s \
  --health-retries 30 \
  --env POSTGRES_DB=keirouter_test \
  --env POSTGRES_USER=postgres \
  --env POSTGRES_PASSWORD=postgres \
  --publish 127.0.0.1::5432 \
  postgres:16-alpine >/dev/null
POSTGRES_STARTED=true
wait_for_postgres
POSTGRES_PORT="$(docker port "$POSTGRES_NAME" 5432/tcp | awk 'NR == 1 { sub(/^.*:/, ""); print }')"
if [[ -z "$POSTGRES_PORT" ]]; then
  printf 'Could not determine temporary PostgreSQL port.\n' >&2
  exit 1
fi

run 'Test PostgreSQL compatibility' bash -c \
  'cd backend && KEIROUTER_TEST_POSTGRES_DSN="$1" go test ./internal/store -run TestPostgresCompatibility -count=1 -v' \
  _ "postgres://postgres:postgres@127.0.0.1:${POSTGRES_PORT}/keirouter_test?sslmode=disable"

run 'Validate default Compose file' docker compose -f compose.yaml config
run 'Validate PostgreSQL Compose override' docker compose -f compose.yaml -f compose.postgres.yaml config
run 'Validate Coolify PostgreSQL Compose' env \
  KEIROUTER_MASTER_KEY=test-master-key \
  KEIROUTER_DATABASE__DSN='postgres://user:password@postgres:5432/keirouter?sslmode=disable' \
  docker compose -f compose.coolify-postgres.yaml config
run 'Build Docker image' docker build -f deploy/Dockerfile -t keirouter:test .

run 'Start isolated Compose Headroom smoke stack' env KEIROUTER_PORT='127.0.0.1:' \
  docker compose --project-name "$COMPOSE_PROJECT" -f compose.yaml up --detach --build
COMPOSE_STARTED=true
wait_for_compose_service headroom
wait_for_compose_service keirouter
COMPOSE_PORT="$(docker compose --project-name "$COMPOSE_PROJECT" -f compose.yaml port keirouter 20180 | awk 'NR == 1 { sub(/^.*:/, ""); print }')"
if [[ -z "$COMPOSE_PORT" ]]; then
  printf 'Could not determine temporary KeiRouter port.\n' >&2
  exit 1
fi
COMPOSE_URL="http://127.0.0.1:${COMPOSE_PORT}"
COMPOSE_COOKIE="$(mktemp)"
run 'Authenticate to isolated Compose dashboard' curl --fail --silent --show-error \
  --cookie-jar "$COMPOSE_COOKIE" \
  --header 'Content-Type: application/json' \
  --data '{"password":"keirouter"}' \
  "$COMPOSE_URL/api/auth/login"
HEADROOM_PROBE="$(curl --fail --silent --show-error \
  --cookie "$COMPOSE_COOKIE" \
  --header 'Content-Type: application/json' \
  --data '{"timeout_ms":10000}' \
  "$COMPOSE_URL/api/settings/headroom-test")"
if ! grep -Eq '"ok"[[:space:]]*:[[:space:]]*true' <<<"$HEADROOM_PROBE"; then
  printf 'Bundled Headroom dashboard probe failed: %s\n' "$HEADROOM_PROBE" >&2
  exit 1
fi

printf '\nAll local verification checks passed.\n'
