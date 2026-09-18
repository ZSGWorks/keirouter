package gateway

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

// TestAdminSQLiteRestoreRenameFailureLeavesDBIntact verifies that a POSIX
// rename failure returns a plain 500 without running the Windows fallback
// (which closes the DB pool and deletes the live database file).
func TestAdminSQLiteRestoreRenameFailureLeavesDBIntact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only rename failure injection")
	}
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "keirouter.db")

	database, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: dbPath}, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	// Pre-create the safety-backup files the handler would use so its
	// copyFile step succeeds on the read-only directory (writing an existing
	// file needs no directory write permission). Cover the current second and
	// the next one to tolerate the second boundary.
	now := time.Now().UTC()
	for _, offset := range []time.Duration{0, time.Second} {
		safetyPath := dbPath + ".before-restore-" + now.Add(offset).Format("20060102-150405")
		require.NoError(t, os.WriteFile(safetyPath, []byte("placeholder"), 0o600))
	}

	// Build a valid SQLite upload (the handler validates it before renaming).
	uploadPath := filepath.Join(t.TempDir(), "upload.db")
	upload, err := sql.Open("sqlite", uploadPath)
	require.NoError(t, err)
	_, err = upload.Exec("CREATE TABLE probe (x INTEGER)")
	require.NoError(t, err)
	require.NoError(t, upload.Close())
	content, err := os.ReadFile(uploadPath)
	require.NoError(t, err)

	s := &Server{db: database, log: slog.Default()}

	// Read-only data directory: os.Rename into it fails with EACCES.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	rec := postMultipartFile(t, s.adminSQLiteRestore, "upload.db", content)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "replace database failed")

	// The DB pool must still be usable: no fallback ran.
	require.NoError(t, database.SQL().Ping())
}
