---
id: TASK-032
title: Fix review findings in 9router SQLite import integration
status: Done
assignee: []
created_date: '2026-09-17 19:05'
updated_date: '2026-09-17 19:24'
labels:
  - security
  - import
  - bugfix
dependencies: []
type: bug
ordinal: 38000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Fixes from approved review of upstream integration branch (plan: ~/.plannotator/plans/fix-review-findings-9router-sq-2026-09-17-approved.md):

- F1 HIGH: delete9routerRows (backend/internal/gateway/admin_foreign_import_sqlite.go:280-315) deletes endpoint_settings, auth.password_hash, provider_routing_% in overwrite mode without backup -> dashboard lockout + destroys fork-authored routing overrides (settings.go:803 shares keyspace). Settings deletion must be wipe-only.
- F2 MED: usage_records IDs are numeric (strconv.FormatInt) so overwrite deletes nothing (LIKE 'n9:%') and RecordBatch plain INSERT (repo_usage.go:28) fails on PK conflict -> merge re-import errors, overwrite import aborts. Prefix IDs n9: + merge skip-existing.
- F3 MED: sqlite_backup.go:141-159 rename fallback closes DB pool and removes live DB on ANY os.Rename error; gate on windows.
- F4 LOW: bcrypt detection misses $2y$ (PHP-style, computation-identical); rewrite prefix in verifyBcrypt + IsLegacyBcrypt.
- F5 NIT: admin_foreign_import.go:389-392 comment claims upsert; actual behavior is skip-on-conflict (Skipped).

Tests: overwrite preserves settings; usage merge/overwrite idempotency; $2y$ tolerance; restore rename-failure 500 on POSIX.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Overwrite mode no longer deletes endpoint_settings, auth.password_hash, or provider_routing_% keys; wipe mode unchanged
- [x] #2 Usage records carry n9:-prefixed IDs; merge re-import skips existing rows without errors; overwrite re-import replaces n9 usage rows
- [x] #3 sqlite restore rename fallback only runs on windows; POSIX rename failure returns 500 with DB intact
- [x] #4 bcrypt $2y$ hashes verify and lazily re-hash like $2a$/$2b$
- [x] #5 Comment at admin_foreign_import.go:389 describes actual skip-on-conflict behavior
- [x] #6 gofmt clean, make vet + make test green, frontend typecheck green, ./scripts/verify.sh green
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed all 5 review findings on chore/integrate-upstream-9router-sqlite (commit 6a7c42a):

- F1 HIGH: delete9routerRows settings deletion now wipe-only; overwrite preserves endpoint_settings, auth.password_hash, provider_routing_* (lockout fixed).
- F2 MED: usage records carry n9:-prefixed IDs; merge mode skips existing via pre-queried id set; overwrite now clears previous n9 usage rows cleanly (no PK-conflict abort).
- F3 MED: sqlite restore rename fallback gated on runtime.GOOS == "windows"; POSIX failures return plain 500 with pool intact (test injects EACCES via read-only dir).
- F4 LOW: crypto.VerifyPassword dispatches via IsLegacyBcrypt, which now accepts $2y$; verifyBcrypt rewrites $2y$ -> $2a$ before compare; lazy re-hash works.
- F5 NIT: admin_foreign_import.go comment corrected (skip-on-conflict semantics).

Tests added: TestDelete9routerRowsOverwritePreservesSettings, TestImport9routerUsageMergeSkipsExisting, TestImport9routerUsageOverwriteReplaces, TestVerifyPasswordAccepts2yBcrypt, TestAdminSQLiteRestoreRenameFailureLeavesDBIntact. UI copy notes settings-merge behavior in overwrite mode.

Verification: gofmt clean, go vet clean, go test ./... green, frontend typecheck green, ./scripts/verify.sh all checks passed.
<!-- SECTION:FINAL_SUMMARY:END -->
