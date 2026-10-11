package models

import (
	"context"

	"gorm.io/gorm"
)

// txKey is the unexported context-key type for the active GORM transaction.
// Keeping it unexported means no other package can collide with or overwrite
// the key.
type txKey struct{}

// WithTx returns a child context carrying the active GORM transaction handle.
// Repositories read this via DBFrom so that callers do not need to thread a
// transactional manager through every method signature.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// DBFrom returns the *gorm.DB attached to ctx, or fallback when none is
// attached. Repositories call DBFrom(ctx, r.db) so that calls made inside a
// Transactor.WithinTx block automatically pick up the transaction handle.
func DBFrom(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}

	return fallback
}

// inTransaction reports whether db's connection is an open transaction. It
// asks the way GORM itself does: by whether the connection can commit.
func inTransaction(db *gorm.DB) bool {
	if db == nil || db.Statement == nil {
		return false
	}

	committer, ok := db.Statement.ConnPool.(gorm.TxCommitter)

	return ok && committer != nil
}
