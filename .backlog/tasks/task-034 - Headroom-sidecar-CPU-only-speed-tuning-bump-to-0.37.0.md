---
id: TASK-034
title: Headroom sidecar CPU-only speed tuning + bump to 0.37.0
status: Done
assignee: []
created_date: '2026-09-18 14:24'
updated_date: '2026-09-18 14:34'
labels: []
dependencies: []
modified_files:
  - compose.yaml
  - compose.coolify-postgres.yaml
  - deploy/README.md
  - README.md
  - backend/internal/headroom/compressor.go
type: enhancement
ordinal: 39000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implement approved plan: CPU-only, no-RSS-growth tuning of the Headroom sidecar; bump image 0.36.0 to 0.37.0; correct incorrect arena/spinning claims in deploy/README.md. See approved plan at .plannotator/plans/headroom-sidecar-cpu-only-spee-2026-09-18-approved.md.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Both compose files pin headroom image 0.37.0
- [x] #2 Both compose files carry the CPU-only tuning env block (group A)
- [x] #3 deploy/README.md arena/spinning claims match onnx_runtime defaults
- [x] #4 Baseline-vs-after RSS and latency recorded
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Pull headroom:0.37.0, re-audit HEADROOM_* env surface + arena/spinning/intra-thread defaults.
2. Measure baseline RSS + compress latency.
3. Bump image tag in compose.yaml and compose.coolify-postgres.yaml.
4. Add group-A CPU tuning env vars to both compose files.
5. Fix deploy/README.md arena/spinning wording; document recommended CPU-only set.
6. Restart sidecar, verify env/readyz/RSS/latency.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
- Re-audited 0.37.0 in throwaway container before applying: arena False, spinning False (same as 0.36). New env vars in 0.37 are compression-cache/logging related (HEADROOM_COMPRESSION_CACHE_*, HEADROOM_LOG_LEVEL, etc.), none in the chosen CPU-only set.
- Dropped planned OMP_NUM_THREADS/MKL_NUM_THREADS/OPENBLAS_NUM_THREADS=1: upstream already does `os.environ.setdefault(var, str(n))` with n=1 in `memory/adapters/embedders.py:_init_cpu_embed_worker` (#198), so setting them in compose was redundant.
- Replaced hardcoded intra-thread 12 with empty passthrough `${KEIROUTER_HEADROOM_KOMPRESS_INTRA_THREADS:-}` so ONNX Runtime keeps its portable automatic choice (physical cores) while still overridable.
- Big RSS drop (1.9 -> 1.0 GiB) came with the 0.37.0 image itself, not the tuning env; docs updated to the observed plateau.
- 0.37.0 was already pulled and the running sidecar recreated via `docker compose up -d headroom`.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Bumped Headroom sidecar `ghcr.io/headroomlabs-ai/headroom` 0.36.0 -> 0.37.0 and added a CPU-only, no-RSS-growth tuning block to both compose files, plus corrected inaccurate arena/spinning docs.

Changes:
- `compose.yaml`, `compose.coolify-postgres.yaml`: image 0.37.0; env block adds `HEADROOM_KOMPRESS_BACKEND=onnx`, `HEADROOM_KOMPRESS_ONNX_FILENAME=onnx/kompress-int8-wo.onnx`, `HEADROOM_KOMPRESS_ONNX_INTER_THREADS=1`, `HEADROOM_KOMPRESS_ONNX_INTRA_THREADS=${KEIROUTER_HEADROOM_KOMPRESS_INTRA_THREADS:-}` (empty = ORT auto, portable), `HEADROOM_KOMPRESS_MAX_CONCURRENT=1`, `HEADROOM_COMPRESSION_MAX_WORKERS=6`. ONNX arena + thread-spinning deliberately left off.
- `deploy/README.md`: fixed wrong claim that arena/thread-spinning are on by default and do not affect memory; documented the tuning block. Memory figures corrected 1.5-2 GB -> 1-1.5 GB.
- `README.md`: Headroom cross-ref number aligned.
- `backend/internal/headroom/compressor.go`: comment only, version range v0.36-v0.37.

Re-audit vs 0.36.0: arena/spinning defaults unchanged (both off); 11 new env vars in 0.37.0, none affecting the chosen set; kompress defaults (int8-wo, batch 32, onnx max_concurrent 1) unchanged.

Measured (host 12 physical cores, same payload, 10 sequential warm calls):
- 0.36.0: median 15.1 ms (min 13.7 / max 21.0), RSS 1.905 GiB.
- 0.37.0 tuned: median 14.3 ms (min 12.4 / max 17.8), RSS 1.011 GiB stable under 60-load + idle.
- Compression behavior unchanged: tokens 814 -> 758 in both.

Verification: `docker compose config` OK both files; `/readyz` healthy 0.37.0; no env warnings; `gofmt` clean; `go test ./backend/internal/headroom/...` ok; `./scripts/verify.sh` all checks passed; Code Health safeguard passed.
<!-- SECTION:FINAL_SUMMARY:END -->
