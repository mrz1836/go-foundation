package db_test

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/db"
)

// openSQLiteRaw opens a SQLite connection at dsn with no schema, silencing the
// GORM logger and closing the handle at test end. maxOpen, when positive, caps
// the pool so the :memory: single-writer shape can be reproduced exactly.
func openSQLiteRaw(t *testing.T, dsn string, maxOpen int) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err, "open %s", dsn)
	if maxOpen > 0 {
		sqlDB, derr := gdb.DB()
		require.NoError(t, derr)
		sqlDB.SetMaxOpenConns(maxOpen)
	}
	t.Cleanup(func() {
		if sqlDB, derr := gdb.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	return gdb
}

// fileDSN builds a file-backed SQLite DSN under a fresh temp dir, so each case
// gets its own database and the WAL sidecar files are cleaned up with the dir.
func fileDSN(t *testing.T, pragmas string) string {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "pragma.db")
	if pragmas != "" {
		dsn += "?" + pragmas
	}
	return dsn
}

// TestVerifySQLitePragmasRejectsFileWithoutWAL proves a file database left on the
// default rollback journal is refused, and the error names the pragma so the
// operator knows what to set.
func TestVerifySQLitePragmasRejectsFileWithoutWAL(t *testing.T) {
	t.Parallel()
	gdb := openSQLiteRaw(t, fileDSN(t, ""), 0)

	err := db.VerifySQLitePragmas(gdb)
	require.ErrorIs(t, err, db.ErrSQLitePragma)
	assert.Contains(t, err.Error(), "journal_mode", "the error names the offending pragma")
}

// TestVerifySQLitePragmasRejectsZeroBusyTimeout proves a WAL file database with
// the busy timeout disabled is refused — it would fail a brief write lock
// outright rather than absorbing it.
func TestVerifySQLitePragmasRejectsZeroBusyTimeout(t *testing.T) {
	t.Parallel()
	gdb := openSQLiteRaw(t, fileDSN(t, "_pragma=journal_mode(WAL)&_pragma=busy_timeout(0)"), 0)

	err := db.VerifySQLitePragmas(gdb)
	require.ErrorIs(t, err, db.ErrSQLitePragma)
	assert.Contains(t, err.Error(), "busy_timeout")
}

// TestVerifySQLitePragmasRejectsSynchronousOff proves synchronous=OFF is refused:
// a durable workload must not risk losing a committed write on power loss.
func TestVerifySQLitePragmasRejectsSynchronousOff(t *testing.T) {
	t.Parallel()
	gdb := openSQLiteRaw(t, fileDSN(t,
		"_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(OFF)"), 0)

	err := db.VerifySQLitePragmas(gdb)
	require.ErrorIs(t, err, db.ErrSQLitePragma)
	assert.Contains(t, err.Error(), "synchronous")
}

// TestVerifySQLitePragmasAcceptsHardenedFile is the happy file path: WAL, a busy
// timeout, and a safe synchronous level all present verify cleanly.
func TestVerifySQLitePragmasAcceptsHardenedFile(t *testing.T) {
	t.Parallel()
	gdb := openSQLiteRaw(t, fileDSN(t,
		"_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"+
			"&_pragma=foreign_keys(1)&_txlock=immediate"), 0)

	require.NoError(t, db.VerifySQLitePragmas(gdb))
}

// TestVerifySQLitePragmasAcceptsInMemoryShapes proves both real in-memory shapes —
// bare :memory: with a single connection, and the library's own shared-cache DSN
// — verify successfully. An in-memory database cannot use WAL, so the check
// exempts journal_mode for it.
func TestVerifySQLitePragmasAcceptsInMemoryShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		dsn     string
		maxOpen int
	}{
		{"bare memory, single connection", ":memory:", 1},
		{"shared-cache memory", "file:pragma-shared?mode=memory&cache=shared", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gdb := openSQLiteRaw(t, tc.dsn, tc.maxOpen)
			require.NoError(t, db.VerifySQLitePragmas(gdb), "an in-memory database must not be rejected")
		})
	}
}

// TestSQLitePragmaHelpersReadValues covers the exported scan helpers directly: a
// text pragma comes back as its string value and an integer pragma as its int.
func TestSQLitePragmaHelpersReadValues(t *testing.T) {
	t.Parallel()
	gdb := openSQLiteRaw(t, fileDSN(t,
		"_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), 0)

	journal, err := db.SQLitePragmaString(gdb, "journal_mode")
	require.NoError(t, err)
	assert.Equal(t, "wal", journal)

	busy, err := db.SQLitePragmaInt(gdb, "busy_timeout")
	require.NoError(t, err)
	assert.Equal(t, 5000, busy)
}
