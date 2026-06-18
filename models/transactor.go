package models

import "context"

// Transactor runs fn inside a database transaction. The ctx passed to fn
// carries the transaction handle, which Repository.* methods read via DBFrom.
// Nested calls to WithinTx use GORM savepoints, so services can compose
// without coordinating who owns the outer transaction.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
