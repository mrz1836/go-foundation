package models

import (
	"context"

	"gorm.io/gorm"
)

// gormTransactor implements Transactor by delegating to gorm.DB.Transaction.
type gormTransactor struct {
	db *gorm.DB
}

// NewTransactor returns a Transactor backed by the given write database. When
// the ctx passed to WithinTx already carries a transaction, GORM creates a
// savepoint instead of starting a new top-level transaction.
func NewTransactor(db *gorm.DB) Transactor {
	return &gormTransactor{db: db}
}

// WithinTx runs fn inside a transaction (or savepoint if one is already
// active in ctx). The ctx passed to fn carries the transaction handle via
// WithTx so that DBFrom returns it.
func (t *gormTransactor) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	db := DBFrom(ctx, t.db)

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
