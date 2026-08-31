
<!-- BACKLOG.MD GUIDELINES START -->
<!-- backlog.md-instructions-version: 1.50.1 -->
<CRITICAL_INSTRUCTION>

## Backlog.md Workflow

This project uses Backlog.md for task and project management.

**For every user request in this project, run `backlog instructions overview` before answering or taking action.**

Use the overview to decide whether to search, read, create, or update Backlog tasks.

Before task lifecycle actions, read the matching detailed guide:
- `backlog instructions task-creation` before creating or splitting tasks
- `backlog instructions task-execution` before planning, changing status or assignee, adding a plan or implementation notes, or implementing task work
- `backlog instructions task-finalization` before checking acceptance criteria, writing final summaries, or moving tasks to terminal statuses

Use `backlog <command> --help` before running unfamiliar commands. Help shows options, fields, and examples.

Do not edit Backlog task, draft, document, decision, or milestone markdown files directly. Use the `backlog` CLI so metadata, relationships, and history stay consistent.

</CRITICAL_INSTRUCTION>
<!-- BACKLOG.MD GUIDELINES END -->

## KeiRouter Map

Self-hosted AI gateway. OpenAI/Anthropic-compatible proxy; provider routing,
fallback chains, cache, budgets, guardrails, encrypted credentials. Go server;
React dashboard; SQLite default, PostgreSQL optional.

### Paths

- `backend/cmd/keirouter/`: CLI/server entry. `start`, `tray`, `status`, `bootstrap`.
- `backend/internal/app/`: dependency wiring, migrations, lifecycle, dashboard assets.
- `backend/internal/gateway/`: HTTP API, admin/dashboard, proxy endpoints.
- `backend/internal/connectors/`, `dispatch/`, `transform/`, `pipeline/`: providers, routing/failover, protocol conversion, request flow.
- `backend/internal/store/`, `config/`: persistence, migrations, YAML/environment config.
- `backend/internal/{crypto,vault,auth,identity,oauth}/`: credentials and access control. Security-sensitive.
- `backend/internal/{cache,budget,limits,meter,health,guardrails,observ}/`: operations, quotas, telemetry, safety.
- `frontend/src/`: React 19 + TypeScript Vite dashboard. `pages/`, `components/`, `contexts/`, `lib/`.
- `@keirouter-opencode-plugin/`: independent Node 22+ TypeScript OpenCode plugin.
- `skills/`: agent `SKILL.md` gateway integrations.
- `deploy/`, `compose*.yaml`: production image, SQLite/Postgres/Presidio deployments.
- `.github/workflows/`: issue and pull-request templates only; no CI, release, or Docker publishing automation.

### Local Run

```sh
make install             # frontend deps + Go modules
make dev                 # backend :20180; Vite dashboard :5180
make build               # frontend assets + Go binary
make test                # backend: go test ./...
make vet                 # backend: go vet ./...
./scripts/verify.sh      # former CI-equivalent local verification
cd frontend && npm run typecheck
cd @keirouter-opencode-plugin && npm test && npm run build
```

Frontend Vite proxy sends API calls to backend `:20180`. Before handoff, run
`./scripts/verify.sh` for changes spanning backend, frontend, deployment, or
workflow-equivalent validation. It installs locked dependencies, starts and removes
a temporary PostgreSQL container, and runs Go, frontend, Compose, and Docker checks.
Plugin edits additionally require `cd @keirouter-opencode-plugin && npm test && npm run build`.
No `frontend` lint script exists; do not run or document one.

### Change Rules

- Fork maintenance: point user-facing operational ownership (URLs, images, installers,
  skills, and update targets) at `ZSGWorks/keirouter`. Preserve the Go module/import
  path, repository layout, and `upstream` remote so upstream merges remain mechanical.
- Go: keep tests beside code; run `gofmt`, `make vet`, `make test` for backend changes.
- Frontend: strict TypeScript; run `npm run typecheck`; build when assets/routes change.
- Plugin: run its own tests/build after plugin edits.
- Config: use `config.example.yaml`; environment overrides use `KEIROUTER_*` with `__` nesting.
- Never commit `.env*`, keys, `master.key`, SQLite files, `data/`, `.keirouter/`, `frontend/dist/`, or `backend/internal/webui/dist/`.
- Treat crypto, vault, auth, identity, OAuth, gateway, and proxy edits as security work. Preserve encryption, authorization, SSRF, and credential-forwarding boundaries.
