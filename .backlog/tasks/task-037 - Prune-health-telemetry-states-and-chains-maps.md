---
id: TASK-037
title: Prune health telemetry states and chains maps
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 16:25'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/health/telemetry.go
modified_files:
  - backend/internal/health/telemetry.go
priority: high
type: enhancement
ordinal: 42000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Health telemetry creates a permanent keyState per (provider, account, model, capability) and a chainState per chain in maps that are never pruned. The snapshot/prune passes delete expired buckets but never the map entry itself, so every key ever seen retains history up to the 24h window. On a busy gateway with account/model churn this grows without bound.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Key states with no buckets inside the history window are deleted during the prune pass
- [x] #2 Chain states with no buckets inside the history window are deleted during the prune pass
- [x] #3 flushCurrent no longer iterates dead keys indefinitely
- [x] #4 A test grows distinct keys over time and asserts the maps stay bounded
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. collectSnapshotWrites (holds s.mu): after bucket prune loop, delete states entries whose buckets map is empty — all surviving buckets are snapshotted before delete, so nothing unpersisted is lost; quiet keys get fresh state on next event\n2. pruneChainBuckets: delete chains entries whose buckets are empty\n3. flushCurrent then iterates only live keys (AC#3)\n4. Test: ingest events for many distinct keys, age them past MaxHistoryWindow, flushSnapshots, assert len(service.states)==0 / len(service.chains)==0 and ProviderStatsSince empty; plus a live key survives\n5. gofmt, make vet, go test ./..., CodeHealth telemetry.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Health telemetry now prunes dead entries entirely: collectSnapshotWrites drops keyState entries whose buckets have all aged out of MaxHistoryWindow (buckets are only deleted after being snapshotted, so nothing unpersisted is lost), and pruneChainBuckets removes chains left without retained buckets. Quiet keys/chains recreate fresh state on the next event; markFinalFailure already tolerates missing keys. flushCurrent therefore iterates only live keys. Updated existing prune test (state entry now removed after a second flush pass) and added TestHealthTelemetryDropsQuietKeysAndChains: 25 distinct keys+chains aged past the window are fully pruned while a live key survives. gofmt/make vet/full go test clean. CodeHealth 7.63 vs 7.65 baseline (-0.02, added branch); pre-existing yellow complexity on collectSnapshotWrites unchanged.
<!-- SECTION:FINAL_SUMMARY:END -->
