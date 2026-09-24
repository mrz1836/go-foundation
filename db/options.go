package db

import "gorm.io/gorm"

// options holds the optional GORM behaviors a caller can toggle for a connection
// built by NewConnection. The zero value preserves GORM's defaults, so a
// NewConnection call with no Option leaves behavior exactly as before.
type options struct {
	skipDefaultTransaction bool
	prepareStmt            bool
}

// Option configures an optional behavior applied to the gorm.Config a connection
// is opened with. Options are additive and evaluated in order; the empty set
// leaves GORM's defaults in place. Pass them to NewConnection.
type Option func(*options)

// WithSkipDefaultTransaction toggles gorm.Config.SkipDefaultTransaction (disabled
// by default).
//
// GORM wraps every single Create/Update/Delete in an implicit transaction
// (BEGIN … COMMIT) unless this is enabled. Skipping that wrapper removes two
// round trips per write — a measurable win when the database is reached over a
// network hop — and single statements still commit atomically on their own. The
// only behavior it changes is the implicit all-or-nothing of a cascading Create
// that inserts a parent and its associations in one call: without the wrapping
// transaction, a failure part way through can leave the parent committed. Enable
// it when writes are single-row, or when multi-statement atomicity is managed
// explicitly (for example via gorm.DB.Transaction).
func WithSkipDefaultTransaction(skip bool) Option {
	return func(o *options) { o.skipDefaultTransaction = skip }
}

// WithPrepareStmt toggles gorm.Config.PrepareStmt (disabled by default), GORM's
// prepared-statement cache. When enabled, GORM prepares each distinct statement
// once per connection and reuses it, avoiding re-parsing on repeated queries.
//
// Caveat: do not enable this behind a connection pooler or proxy that multiplexes
// below the session level. PgBouncer in transaction or statement pooling mode
// cannot carry protocol-level prepared statements, and AWS RDS Proxy pins the
// client to a single backend connection for the life of a prepared statement,
// defeating the proxy's connection reuse. Enable it only for a direct (or
// session-pooled) connection.
func WithPrepareStmt(prepare bool) Option {
	return func(o *options) { o.prepareStmt = prepare }
}

// resolveOptions folds opts into a single options value.
func resolveOptions(opts []Option) options {
	var resolved options
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}

	return resolved
}

// gormConfig builds the gorm.Config the resolved options describe.
func (o options) gormConfig() *gorm.Config {
	return &gorm.Config{
		SkipDefaultTransaction: o.skipDefaultTransaction,
		PrepareStmt:            o.prepareStmt,
	}
}
