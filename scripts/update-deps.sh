#!/usr/bin/env bash
# Dependency upgrade guard: never pick a release published within the last N days
# (default 7). Shields against freshly hijacked/malicious package versions.
#
# Usage:
#   ./scripts/update-deps.sh            report only (no writes)
#   ./scripts/update-deps.sh --apply    install safe updates
#   ./scripts/update-deps.sh --days 14  custom release-age floor
#
# ponytail: report/apply only; no semver-range rewriting, no lockfile pinning.
# Add range-aware upgrades only if upgrade churn demands it.
set -euo pipefail

DAYS=7
APPLY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --days) case "${2:-}" in ''|*[!0-9]*) echo "--days needs a positive number" >&2; exit 2;; esac; DAYS="$2"; shift 2 ;;
    --apply) APPLY=1; shift ;;
    -h|--help) awk 'NR>1{ if ($0 !~ /^#/) exit; sub(/^# /,""); print }' "$0"; exit 0 ;;
    *) echo "unknown arg: $1 (see --help)" >&2; exit 2 ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NPM_DIRS=("frontend" "@keirouter-opencode-plugin")

npm_apply() { # dir pkg target
  local d="$ROOT/$1"
  cp "$d/package.json" "$d/.udeps-bak.json"
  if [ -f "$d/package-lock.json" ]; then cp "$d/package-lock.json" "$d/.udeps-bak.lock"; fi
  if (cd "$d" && npm install "$2@$3" --no-audit --no-fund); then
    rm -f "$d/.udeps-bak.json" "$d/.udeps-bak.lock"
  else
    mv "$d/.udeps-bak.json" "$d/package.json"
    if [ -f "$d/.udeps-bak.lock" ]; then mv "$d/.udeps-bak.lock" "$d/package-lock.json"; fi
    echo "  FAILED  $2 (install error; package.json/lockfile restored)"
  fi
}

go_apply() { # mod ver
  local d="$ROOT/backend"
  cp "$d/go.mod" "$d/.udeps-bak.mod"
  if (cd "$d" && go get "$1@$2"); then
    rm -f "$d/.udeps-bak.mod"
  else
    mv "$d/.udeps-bak.mod" "$d/go.mod"
    echo "  FAILED  $1 (go get error; go.mod restored)"
  fi
}

# stdin: npm view <pkg> time --json ; args: days current wanted latest
# prints: "TARGET <ver>" | "BLOCKED <ver> <date>" | "UNKNOWN"
pick_npm_target() {
  python3 -c '
import json, sys, datetime
try:
    t = json.load(sys.stdin)
except Exception:
    print("UNKNOWN"); raise SystemExit
days, cur, want, lat = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
now = datetime.datetime.now(datetime.timezone.utc)
cut = now - datetime.timedelta(days=days)
def dt(v):
    s = t.get(v)
    if not s: return None
    try: return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError: return None
dl, dw = dt(lat), dt(want)
if dl and dl <= cut:
    print(f"TARGET {lat}")
elif want != cur and dw and dw <= cut:
    print(f"TARGET {want}")
elif dl:
    print(f"BLOCKED {lat} {dl.date()}")
else:
    print("UNKNOWN")
' "$DAYS" "$1" "$2" "$3"
}

scan_npm_dir() {
  local dir="$1"
  echo "== npm: $dir =="
  if [ ! -d "$ROOT/$dir/node_modules" ]; then
    echo "  skipped (node_modules missing; run npm install)"; echo; return
  fi
  local out
  out=$(cd "$ROOT/$dir" && npm outdated --json 2>/dev/null || true)
  if [ -z "$out" ] || [ "$out" = "{}" ]; then
    echo "  up to date"; echo; return
  fi
  local flat
  flat=$(printf '%s' "$out" | python3 -c '
import json, sys
d = json.load(sys.stdin)
for pkg, i in d.items():
    print("|".join([pkg, str(i.get("current", "")), str(i.get("wanted", "")), str(i.get("latest", ""))]))
')
  local pkg cur want lat verdict target pub
  while IFS='|' read -r pkg cur want lat; do
    [ -n "$pkg" ] || continue
    verdict=$(cd "$ROOT/$dir" && npm view "$pkg" time --json 2>/dev/null | pick_npm_target "$cur" "$want" "$lat")
    case "$verdict" in
      TARGET*)
        target=${verdict#TARGET }
        echo "  UPDATE  $pkg  $cur -> $target"
        if [ "$APPLY" = 1 ]; then
          npm_apply "$dir" "$pkg" "$target"
        fi
        ;;
      BLOCKED*)
        pub=$(printf '%s' "$verdict" | awk '{print $3}')
        echo "  BLOCKED $pkg  latest $lat published $pub (< ${DAYS}d)"
        ;;
      *) echo "  UNKNOWN $pkg  latest $lat (no publish date)" ;;
    esac
  done <<< "$flat"
  echo
}

# prints "SAFE" | "BLOCKED <date>" ; args: days published
go_date_verdict() {
  python3 -c '
import sys, datetime
days, pub = int(sys.argv[1]), sys.argv[2]
d = datetime.datetime.fromisoformat(pub.replace("Z", "+00:00"))
cut = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=days)
print("SAFE" if d <= cut else f"BLOCKED {d.date()}")
' "$1" "$2"
}

scan_go() {
  echo "== go modules: backend =="
  local esc info pub verdict mod cur upd line mod_cur n_updates=0 n_blocked=0
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    mod_cur=${line%% \[*}
    upd=${line#*\[}; upd=${upd%%]*}
    mod=${mod_cur% *}
    cur=${mod_cur##* }
    esc=$(python3 -c 'import re,sys; print(re.sub(r"[A-Z]", lambda m: "!" + m.group().lower(), sys.argv[1]))' "$mod")
    info=$(curl -fsSL "https://proxy.golang.org/$esc/@v/$upd.info" 2>/dev/null || true)
    pub=$(printf '%s' "$info" | python3 -c '
import json, sys
try: print(json.load(sys.stdin).get("Time", ""))
except Exception: print("")
')
    if [ -z "$pub" ]; then
      echo "  UNKNOWN $mod  $cur -> $upd (no proxy info)"; continue
    fi
    verdict=$(go_date_verdict "$DAYS" "$pub")
    case "$verdict" in
      SAFE)
        n_updates=$((n_updates + 1))
        echo "  UPDATE  $mod  $cur -> $upd"
        if [ "$APPLY" = 1 ]; then
          go_apply "$mod" "$upd"
        fi
        ;;
      BLOCKED*)
        n_blocked=$((n_blocked + 1))
        echo "  BLOCKED $mod  $upd published ${verdict#BLOCKED } (< ${DAYS}d)"
        ;;
    esac
  done < <(cd "$ROOT/backend" && go list -m -u all 2>/dev/null | grep ' \[' )
  if [ "$APPLY" = 1 ] && [ "$n_updates" -gt 0 ]; then
    (cd "$ROOT/backend" && go mod tidy)
  fi
  echo
}

echo "Dependency scan (releases newer than ${DAYS}d are excluded)$([ "$APPLY" = 1 ] && echo ' — APPLY MODE')"
echo
for d in "${NPM_DIRS[@]}"; do
  scan_npm_dir "$d"
done
scan_go
