// Package foundation is the public, domain-agnostic foundation kit shared across
// mrz1836 Go services and libraries.
//
// The module carries only domain-agnostic building blocks. As the kit is
// assembled it exposes focused sub-packages:
//
//   - config        application/database/logging/AWS configuration types
//   - lambda         AWS Lambda (API Gateway v2) ⇄ net/http adapter
//   - middleware     logging, recovery, and request-ID HTTP middleware
//   - httputil       JSON response and error helpers
//   - pagination     cursor-based list pagination
//   - models         generic BaseModel, Repository, Clock, and transaction helpers
//   - secrets        pluggable secret providers (env, AWS, mock)
//   - db             database connection helpers
//   - health         health-check helpers
//   - observability  structured logging initialization
//   - testutil       generic test helpers (test DB, fixed clock, containers)
//
// Project-specific naming (environment prefixes, database names, health
// messages, infrastructure constants) intentionally lives in the consuming
// services, never in this module.
//
// During initial scaffolding the module exposes only its identity; the
// foundation sub-packages are added as they are extracted.
package foundation

// ModulePath is the canonical Go module path for the foundation kit.
const ModulePath = "github.com/mrz1836/go-foundation"
