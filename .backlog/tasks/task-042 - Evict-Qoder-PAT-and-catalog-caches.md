---
id: TASK-042
title: Evict Qoder PAT and catalog caches
status: Done
assignee: []
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:11'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/connectors/qoder.go
modified_files:
  - backend/internal/connectors/qoder.go
priority: high
type: bug
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Qoder connector stores PAT sessions keyed by raw PAT and a per-user model catalog, and never deletes entries. Expiry is checked only on read, so deleted or re-authenticated accounts retain token, identity, and raw model-config data indefinitely. This is both a memory and a secret-retention issue.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Expired PAT sessions are evicted on write and lookup
- [x] #2 Catalog entries past TTL are pruned
- [x] #3 A test asserts stale entries are removed
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Qoder caches now evict stale entries. PAT session cache: expired sessions are deleted on lookup (fast path drops the stale token before re-minting) and evictExpiredQoderSessions sweeps all expired entries on every write, so deleted/re-authenticated accounts no longer retain job tokens (secret retention fix). Model catalog cache: stale read-path entries are pruned via pruneCatalogEntry (pointer-identity guarded so a concurrently refreshed entry is not dropped), and every catalog write runs pruneStaleCatalog, dropping entries past qoderCatalogTTL (1h). Tests: TestQoderPATCacheEvictsExpiredSessions (fresh survives, expired gone) and TestQoderCatalogPrunesStaleEntries. Extracted prune helpers to keep fetchModelCatalog complexity flat. gofmt/make vet/full go test clean; CodeHealth qoder.go 6.27 (baseline 6.39; remaining delta from new eviction logic; pre-existing yellow smells unchanged).
<!-- SECTION:FINAL_SUMMARY:END -->
