// Package cache provides a generic, thread-safe, two-tier TTL cache for
// validating opaque secrets (API keys, tokens) against a loaded set, using a
// caller-supplied match function so the package stays dependency-free.
//
// The cache keeps two layers of state:
//
//  1. A per-secret validation result (the entries map) so repeated checks for
//     the same raw secret are fast and skip the loader and the match scan.
//  2. The full set of loadable items (the loaded slice), refreshed
//     periodically, so validation can run entirely in memory.
//
// Valid results are cached for a longer TTL while invalid results are cached
// for a short TTL, which bounds the memory an enumeration attack can consume
// without holding bad guesses for long. Raw secrets are never stored: they are
// hashed with SHA-256 before being used as map keys. The package imports only
// the standard library; any comparison cost (for example a password-hash
// comparison) lives entirely in the caller-supplied match function.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Cache configuration defaults.
//
// DefaultTTL defines how long a successful (valid) lookup is kept in the
// in-memory cache before it must be revalidated.
//
// DefaultInvalidTTL defines how long an invalid lookup is cached. This short
// TTL helps protect against brute-force and enumeration attacks by temporarily
// remembering bad secrets but not holding them for long.
//
// DefaultMaxEntries is a safeguard to prevent the cache from growing without
// bounds in the face of many unique secrets (for example, an attack).
const (
	DefaultTTL        = 60 * time.Second // TTL for valid entries
	DefaultInvalidTTL = 5 * time.Second  // Short TTL for invalid entries (DoS protection)
	DefaultMaxEntries = 10000            // Prevent memory bloat from enumeration attacks
)

// Loader supplies the full set of items the cache validates against.
//
// LoadAll returns every item that should be considered a valid match target.
// The loader is responsible for any filtering semantics (for example, omitting
// disabled or expired items); the cache treats whatever is returned as the
// authoritative set. It is called on the slow path when the loaded set is
// missing or stale.
type Loader[T any] interface {
	LoadAll(ctx context.Context) ([]T, error)
}

// entry stores the validation result for a single raw secret.
//
// An entry is created whenever a raw secret is validated, regardless of whether
// it matched. For valid secrets the entry keeps a pointer to the matched item
// so subsequent calls don't need to reload or re-run the match function.
//
// Fields:
//   - valid: true if the raw secret matched a loaded item; false otherwise.
//   - item: the matched item for valid secrets; nil for invalid ones.
//   - checkedAt: the time at which this secret was last evaluated.
//   - ttl: the duration for which this entry should be considered fresh.
//     Typically the cache's valid TTL for matches or its invalid TTL for
//     non-matches.
type entry[T any] struct {
	valid     bool          // whether the secret matched a loaded item
	item      *T            // the matched item (if valid)
	checkedAt time.Time     // when this entry was created
	ttl       time.Duration // TTL for this specific entry
}

// isExpired reports whether the cache entry is no longer considered fresh.
//
// Input:
//   - now: a reference time (usually time.Now()) against which to compare the
//     entry's age.
//
// Output:
//   - true if now - checkedAt >= ttl, indicating the entry should not be used
//     for answering new validation requests.
//   - false if the entry is still within its TTL.
func (e *entry[T]) isExpired(now time.Time) bool {
	return now.Sub(e.checkedAt) >= e.ttl
}

// Stats holds a snapshot of the current cache state that can be used for
// debugging, observability, or health checks.
//
// Fields:
//   - EntryCount: total number of per-secret validation entries currently in
//     the cache (both valid and invalid).
//   - LoadedCount: number of items currently loaded in memory.
//   - LoadedAge: how long ago the loaded set was last refreshed.
//   - OldestEntry: age of the oldest entry; zero if there are no entries.
//   - ValidEntries: number of entries marked as valid.
//   - InvalidEntries: number of entries marked as invalid.
//
// This struct is read-only from the caller's perspective and is always returned
// as a value, so callers cannot mutate the internal cache by modifying it.
type Stats struct {
	EntryCount     int           // Number of cached validation results
	LoadedCount    int           // Number of loaded items
	LoadedAge      time.Duration // Age of the loaded set
	OldestEntry    time.Duration // Age of the oldest cache entry
	ValidEntries   int           // Count of valid (matched) entries
	InvalidEntries int           // Count of invalid (unmatched) entries
}

// config holds the resolved options for a cache. It is populated from the
// exported defaults and any Option values before the Cache is constructed,
// which keeps Option non-generic (a plain func(*config)) so option call sites
// don't need to spell out the cache's type parameter.
type config struct {
	ttl        time.Duration
	invalidTTL time.Duration
	maxEntries int
	now        func() time.Time
}

// Option is a functional option used to configure a Cache during construction.
//
// Each Option receives a pointer to the internal config and may mutate its
// fields (TTLs, max entries, clock function) before the cache is built. This
// pattern avoids constructors with many positional parameters.
type Option func(*config)

// WithTTL overrides the default TTL used for valid cache entries.
//
// Input:
//   - ttl: duration to use for future valid entries.
//
// Behavior:
//   - Does not affect existing entries, only entries created after the cache is
//     constructed.
//   - Can be chained with other Option values in New.
func WithTTL(ttl time.Duration) Option {
	return func(c *config) {
		c.ttl = ttl
	}
}

// WithInvalidTTL overrides the default TTL used for invalid cache entries.
//
// Input:
//   - ttl: duration to use for future invalid entries. A short value limits how
//     long the cache remembers bad secrets, which bounds enumeration-attack
//     memory while still absorbing repeated bad guesses.
//
// Behavior:
//   - Does not affect existing entries, only entries created after the cache is
//     constructed.
func WithInvalidTTL(ttl time.Duration) Option {
	return func(c *config) {
		c.invalidTTL = ttl
	}
}

// WithMaxEntries overrides the default maximum number of cached validation
// entries.
//
// Input:
//   - n: the maximum number of entries to retain before the cache evicts
//     expired and, if necessary, the oldest entries to make room.
//
// Behavior:
//   - Bounds the memory the entries map can consume in the face of many unique
//     secrets.
func WithMaxEntries(n int) Option {
	return func(c *config) {
		c.maxEntries = n
	}
}

// WithNowFunc injects a custom clock function into the cache.
//
// This is primarily intended for tests, where deterministic control over time
// is important. In production, the default is time.Now.
//
// Input:
//   - fn: a function returning the current time to use for TTL comparisons and
//     age calculations.
func WithNowFunc(fn func() time.Time) Option {
	return func(c *config) {
		c.now = fn
	}
}

// Cache provides thread-safe in-memory caching for opaque-secret validation.
//
// It has two main responsibilities:
//  1. Cache per-secret validation results in the entries map so that repeated
//     checks for the same raw secret are fast and avoid reloading or re-running
//     the match function.
//  2. Cache the full loaded set in the loaded slice, refreshing it
//     periodically, so that validation can run entirely in memory.
//
// Concurrency:
//   - All state is guarded by the mu RWMutex.
//   - Read-heavy operations use RLock for fast-path cache hits, while write
//     operations acquire the full Lock when modifying maps/slices.
//
// Dependencies:
//   - loader: supplies the full set of items to validate against when the
//     loaded set is stale or missing.
//   - match: reports whether a raw secret matches a loaded item; all
//     comparison cost lives here, keeping the cache dependency-free.
//   - now: injectable clock function used for TTL and age calculations;
//     defaults to time.Now but can be overridden in tests.
//
// The zero value is not safe for direct use; New must be used to initialize the
// maps and defaults.
type Cache[T any] struct {
	mu       sync.RWMutex
	entries  map[string]*entry[T] // key: SHA-256 of raw secret, value: validation result
	loaded   []T                  // full loaded set
	loadedAt time.Time            // when the loaded set was last refreshed

	ttl        time.Duration // default TTL for valid entries
	invalidTTL time.Duration // TTL for invalid entries
	maxEntries int           // maximum number of cached validation entries

	// Dependencies (injectable for testing)
	loader Loader[T]
	match  func(raw string, item T) bool
	now    func() time.Time // injectable time function for testing
}

// New constructs and returns a fully initialized Cache.
//
// Inputs:
//   - l: the Loader used to load the full set of items to validate against when
//     the loaded set must be refreshed.
//   - match: a function reporting whether a raw secret matches a loaded item.
//     All comparison cost (for example a password-hash comparison) lives here,
//     so the package itself stays dependency-free.
//   - opts: optional functional options (for example WithTTL, WithInvalidTTL,
//     WithMaxEntries, WithNowFunc) that customize the cache's behavior.
//
// Output:
//   - *Cache[T]: a ready-to-use cache instance with internal maps allocated and
//     defaults applied.
//
// The returned cache does not preload anything; the first validation request
// triggers a call to the loader.
func New[T any](l Loader[T], match func(raw string, item T) bool, opts ...Option) *Cache[T] {
	cfg := config{
		ttl:        DefaultTTL,
		invalidTTL: DefaultInvalidTTL,
		maxEntries: DefaultMaxEntries,
		now:        time.Now,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &Cache[T]{
		entries:    make(map[string]*entry[T]),
		loaded:     nil,
		ttl:        cfg.ttl,
		invalidTTL: cfg.invalidTTL,
		maxEntries: cfg.maxEntries,
		loader:     l,
		match:      match,
		now:        cfg.now,
	}
}

// Validate checks whether the provided raw secret is currently recognized as
// valid according to the cached state (and, if necessary, the loader).
//
// Inputs:
//   - ctx: context for cancellation and deadlines passed down to the loader
//     when refreshing the loaded set.
//   - rawKey: the plain-text secret provided by the caller (for example, from
//     an HTTP header).
//
// Outputs:
//   - *T: the matching item if the secret is valid; nil if the secret is
//     invalid or not found.
//   - error: non-nil only if there was an issue loading the set from the
//     loader. A nil error with a nil *T indicates a clean "not found" result.
//
// Behavior:
//   - Fast path: first attempts a read-only lookup in the entries map. If a
//     non-expired entry is found, it is returned immediately.
//   - Slow path: on cache miss or expired entry, acquires a write lock,
//     refreshes the loaded set if needed, runs the match function against each
//     loaded item, caches the result, and returns it.
//
//nolint:nilnil // Returning (nil, nil) is intentional - a nil item with a nil error signals "not found" without an error
func (c *Cache[T]) Validate(ctx context.Context, rawKey string) (*T, error) {
	now := c.now()
	cacheKey := hashKey(rawKey)

	// Fast path: read lock only, check if we have a fresh cached result
	c.mu.RLock()
	e, exists := c.entries[cacheKey]
	loadedValid := !c.loadedAt.IsZero() && now.Sub(c.loadedAt) < c.ttl
	c.mu.RUnlock()

	// Cache hit with fresh entry
	if exists && !e.isExpired(now) {
		if e.valid {
			return e.item, nil
		}
		return nil, nil
	}

	// Cache miss or stale - need write lock
	return c.validateAndCache(ctx, rawKey, cacheKey, loadedValid, now)
}

// Invalidate clears all cached entries and forces a fresh load on the next
// validation call.
//
// Use this method when you know the set of valid secrets has changed materially
// (for example, after a key rotation, revocation, or bulk update) and you want
// subsequent validations to reflect those changes immediately.
//
// Behavior:
//   - Clears per-secret validation results in entries.
//   - Discards the loaded set.
//   - Resets the loaded timestamp to the zero time, causing the next validation
//     to reload from the loader.
func (c *Cache[T]) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*entry[T])
	c.loaded = nil
	c.loadedAt = time.Time{} // Zero time forces refresh on next validate
}

// InvalidateKey removes a specific raw secret from the validation cache.
//
// Input:
//   - rawKey: the plain-text secret whose cached validation result should be
//     discarded.
//
// Behavior:
//   - Next time Validate is called with this secret, the cache will perform a
//     full validation flow (including the match scan) instead of using any
//     previously stored result.
func (c *Cache[T]) InvalidateKey(rawKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, hashKey(rawKey))
}

// Stats returns a point-in-time snapshot of the cache's internal metrics.
//
// Output:
//   - Stats: a value populated with counts and ages computed at the moment
//     Stats is called.
//
// Concurrency:
//   - Uses a read lock so that stats can be gathered concurrently with
//     in-flight validation operations.
func (c *Cache[T]) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := c.now()
	stats := Stats{
		EntryCount:  len(c.entries),
		LoadedCount: len(c.loaded),
	}

	if !c.loadedAt.IsZero() {
		stats.LoadedAge = now.Sub(c.loadedAt)
	}

	for _, e := range c.entries {
		age := now.Sub(e.checkedAt)
		if age > stats.OldestEntry {
			stats.OldestEntry = age
		}
		if e.valid {
			stats.ValidEntries++
		} else {
			stats.InvalidEntries++
		}
	}

	return stats
}

// validateAndCache performs the full validation flow for a raw secret and
// stores the result in the cache.
//
// This method is invoked only when the fast path in Validate either missed the
// cache or found a stale entry, and it always runs under a write lock to avoid
// race conditions while mutating internal state.
//
// Inputs:
//   - ctx: context used when reloading items from the loader.
//   - rawKey: the plain-text secret being validated.
//   - cacheKey: the SHA-256 hash of rawKey, used as the entries map key.
//   - loadedValid: a hint computed before acquiring the write lock that
//     indicates whether the current loaded set is likely still fresh. The
//     method may still choose to refresh based on its own checks.
//   - now: timestamp to use for TTL, age, and entry bookkeeping.
//
// Outputs:
//   - *T: the matching item if the secret is valid; nil if invalid.
//   - error: non-nil only if an attempt to refresh the loaded set fails.
//
//nolint:nilnil // Returning (nil, nil) is intentional - a nil item with a nil error signals "not found" without an error
func (c *Cache[T]) validateAndCache(ctx context.Context, rawKey, cacheKey string, loadedValid bool, now time.Time) (*T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have updated)
	if e, exists := c.entries[cacheKey]; exists && !e.isExpired(now) {
		if e.valid {
			return e.item, nil
		}
		return nil, nil
	}

	// Refresh the loaded set if stale or never loaded
	if !loadedValid || c.loadedAt.IsZero() || now.Sub(c.loadedAt) >= c.ttl {
		if err := c.refreshLocked(ctx, now); err != nil {
			return nil, err
		}
	}

	// Enforce max cache size before adding new entry
	if len(c.entries) >= c.maxEntries {
		c.evictOldestLocked(now)
	}

	// Validate against the loaded set (via the match function)
	result := c.validateAgainstLoadedLocked(rawKey, now)

	// Cache the result
	c.entries[cacheKey] = result

	if result.valid {
		return result.item, nil
	}
	return nil, nil
}

// refreshLocked reloads the full set of items from the loader and updates the
// in-memory loaded slice.
//
// Inputs:
//   - ctx: context used for the loader call; cancellation or timeout propagate
//     to the underlying LoadAll operation.
//   - now: timestamp recorded as the loaded time to indicate when the refresh
//     occurred.
//
// Outputs:
//   - error: non-nil if the loader call fails; in that case the cache keeps its
//     previous loaded set and the caller will see the error.
//
// Concurrency:
//   - Must be called with the cache's write lock held; it mutates loaded and
//     the loaded timestamp directly.
func (c *Cache[T]) refreshLocked(ctx context.Context, now time.Time) error {
	items, err := c.loader.LoadAll(ctx)
	if err != nil {
		return err
	}

	// Copy into a fresh slice to avoid retaining the loader's backing array
	c.loaded = make([]T, len(items))
	copy(c.loaded, items)
	c.loadedAt = now

	return nil
}

// validateAgainstLoadedLocked attempts to match the provided raw secret against
// every item in the loaded set.
//
// Input:
//   - rawKey: the plain-text secret to compare against loaded items.
//   - now: timestamp used when constructing the resulting entry.
//
// Output:
//   - *entry[T]: a new entry describing whether the secret was valid. If a match
//     is found, the entry is marked valid, contains a copy of the matching item,
//     and uses the cache's valid TTL. If no match is found, the entry is marked
//     invalid, has a nil item, and uses the cache's invalid TTL.
//
// Concurrency:
//   - Must be called with the cache's write lock held since it reads from
//     loaded, which may be mutated elsewhere under the same lock.
func (c *Cache[T]) validateAgainstLoadedLocked(rawKey string, now time.Time) *entry[T] {
	for i := range c.loaded {
		if c.match(rawKey, c.loaded[i]) {
			// Copy the element to avoid slice-element reference issues
			it := c.loaded[i]
			return &entry[T]{
				valid:     true,
				item:      &it,
				checkedAt: now,
				ttl:       c.ttl,
			}
		}
	}

	// No match - cache as invalid with the shorter TTL
	return &entry[T]{
		valid:     false,
		item:      nil,
		checkedAt: now,
		ttl:       c.invalidTTL,
	}
}

// evictOldestLocked enforces the max-entries limit by removing expired and, if
// necessary, the oldest remaining entries.
//
// Input:
//   - now: reference time used to evaluate entry age and expiration.
//
// Behavior:
//   - First removes all entries that have already expired based on their
//     individual TTLs.
//   - If the cache still contains at least maxEntries entries, it then removes
//     the oldest ~10% (at least one) based on checkedAt timestamps.
//
// Concurrency:
//   - Must be called with the cache's write lock held since it mutates the
//     entries map.
func (c *Cache[T]) evictOldestLocked(now time.Time) {
	c.removeExpiredEntries(now)

	if len(c.entries) >= c.maxEntries {
		c.removeOldestEntries(now)
	}
}

// removeExpiredEntries scans the entries map and deletes any entry whose TTL has
// elapsed.
//
// Input:
//   - now: reference time used to determine which entries are expired.
//
// Concurrency:
//   - Must be called with the cache's write lock held as it mutates the entries
//     map in place.
func (c *Cache[T]) removeExpiredEntries(now time.Time) {
	for key, e := range c.entries {
		if e.isExpired(now) {
			delete(c.entries, key)
		}
	}
}

// removeOldestEntries removes the oldest subset of entries from the cache when
// it is still at or above its configured capacity after purging already expired
// entries.
//
// Input:
//   - now: reference time used to compute the relative age of each entry.
//
// Behavior:
//   - Computes the age of each remaining entry and then removes the oldest ~10%
//     (rounded up to at least one entry). A simple selection algorithm is used
//     here since the cache size is bounded.
//
// Concurrency:
//   - Must be called with the cache's write lock held because it iterates over
//     and deletes from the entries map.
func (c *Cache[T]) removeOldestEntries(now time.Time) {
	toRemove := len(c.entries) / 10
	if toRemove < 1 {
		toRemove = 1
	}

	type keyAge struct {
		key string
		age time.Duration
	}

	oldest := make([]keyAge, 0, len(c.entries))
	for key, e := range c.entries {
		oldest = append(oldest, keyAge{key: key, age: now.Sub(e.checkedAt)})
	}

	// Selection sort to find the oldest entries
	for i := 0; i < toRemove && i < len(oldest); i++ {
		maxIdx := i
		for j := i + 1; j < len(oldest); j++ {
			if oldest[j].age > oldest[maxIdx].age {
				maxIdx = j
			}
		}
		oldest[i], oldest[maxIdx] = oldest[maxIdx], oldest[i]
		delete(c.entries, oldest[i].key)
	}
}

// hashKey returns a SHA-256 hex digest of the raw secret for use as an entries
// map key. This avoids storing plain-text secrets in process memory.
func hashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
