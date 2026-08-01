package db

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ErrSQLitePragma reports that a SQLite connection is missing a pragma the
// caller requires. VerifySQLitePragmas wraps it, naming the offending pragma, so
// errors.Is(err, ErrSQLitePragma) matches while the message stays specific.
var ErrSQLitePragma = errors.New("sqlite: connection is missing a required pragma")

// VerifySQLitePragmas verifies the connection pragmas a concurrent SQLite
// workload relies on, returning ErrSQLitePragma (naming the pragma) on a hard
// failure.
//
// A file database must be in WAL so readers do not block a writer; an in-memory
// database — which cannot use WAL — is exempt from that one requirement but,
// like any database, still needs a positive busy_timeout to absorb a brief write
// lock and a safe synchronous level so a committed write survives a crash.
// synchronous is required to be NORMAL(1) or FULL(2); OFF(0) is refused.
//
// The check reads pragmas only, so it needs no dialector and makes no
// assumptions about how the connection was opened. Warnings a caller may want on
// top of the fatal checks — foreign_keys, DSN-only settings such as
// _txlock=immediate — are left to the caller, which knows its own schema.
func VerifySQLitePragmas(gdb *gorm.DB) error {
	journalMode, err := SQLitePragmaString(gdb, "journal_mode")
	if err != nil {
		return fmt.Errorf("%w: reading journal_mode: %w", ErrSQLitePragma, err)
	}
	inMemory := strings.EqualFold(journalMode, "memory")
	if !inMemory && !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf(
			"%w: journal_mode is %q, want wal — open with the _pragma=journal_mode(WAL) DSN parameter",
			ErrSQLitePragma, journalMode,
		)
	}

	busyTimeout, err := SQLitePragmaInt(gdb, "busy_timeout")
	if err != nil {
		return fmt.Errorf("%w: reading busy_timeout: %w", ErrSQLitePragma, err)
	}
	if busyTimeout <= 0 {
		return fmt.Errorf(
			"%w: busy_timeout is %d, want > 0 — open with the _pragma=busy_timeout(5000) DSN parameter",
			ErrSQLitePragma, busyTimeout,
		)
	}

	synchronous, err := SQLitePragmaInt(gdb, "synchronous")
	if err != nil {
		return fmt.Errorf("%w: reading synchronous: %w", ErrSQLitePragma, err)
	}
	// 1 = NORMAL, 2 = FULL are safe; 0 = OFF risks losing a committed write on
	// power loss, which for a durable workload is exactly the failure it exists
	// to prevent.
	if synchronous != 1 && synchronous != 2 {
		return fmt.Errorf(
			"%w: synchronous is %d, want NORMAL(1) or FULL(2), not OFF(0)",
			ErrSQLitePragma, synchronous,
		)
	}

	return nil
}

// SQLitePragmaString reads a text-valued PRAGMA from gdb (for example
// journal_mode). The pragma name is a caller-controlled constant, not user
// input.
func SQLitePragmaString(gdb *gorm.DB, name string) (string, error) {
	var v string
	err := gdb.Raw("PRAGMA " + name).Row().Scan(&v)
	return v, err
}

// SQLitePragmaInt reads an integer-valued PRAGMA from gdb (for example
// busy_timeout or synchronous). The pragma name is a caller-controlled constant,
// not user input.
func SQLitePragmaInt(gdb *gorm.DB, name string) (int, error) {
	var v int
	err := gdb.Raw("PRAGMA " + name).Row().Scan(&v)
	return v, err
}
