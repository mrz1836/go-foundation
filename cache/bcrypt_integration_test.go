package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrz1836/go-foundation/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// apiKey mirrors the credential record a real authorizer caches: an identifier,
// a bcrypt hash of the secret, and an active flag.
type apiKey struct {
	ID      string
	KeyHash string
	Active  bool
}

// bcryptLoader mirrors a repository that returns only active credentials. It
// stores every added key but LoadAll filters to active ones, exactly like a
// "find all active" query, and counts how many times it was called.
type bcryptLoader struct {
	mu        sync.RWMutex
	keys      []apiKey
	callCount int64
}

// LoadAll returns only the active keys and bumps the call counter.
func (l *bcryptLoader) LoadAll(_ context.Context) ([]apiKey, error) {
	atomic.AddInt64(&l.callCount, 1)

	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]apiKey, 0, len(l.keys))
	for _, k := range l.keys {
		if k.Active {
			result = append(result, k)
		}
	}
	return result, nil
}

// add stores a credential, hashing the plaintext secret with bcrypt (MinCost
// for fast tests) just as a real caller would when persisting a key.
func (l *bcryptLoader) add(id, plaintext string, active bool) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)

	l.mu.Lock()
	l.keys = append(l.keys, apiKey{ID: id, KeyHash: string(hash), Active: active})
	l.mu.Unlock()
}

func (l *bcryptLoader) getCallCount() int64 {
	return atomic.LoadInt64(&l.callCount)
}

// integrationClock is a minimal fixed clock so the integration test drives the
// public WithNowFunc seam without depending on wall-clock time.
type integrationClock struct {
	current time.Time
}

func (c *integrationClock) Now() time.Time {
	return c.current
}

// TestBcryptMatcher_Integration proves the generic seam end-to-end with a real
// bcrypt match function, mirroring the way an API-key authorizer uses the cache.
// It drives only the exported cache API.
func TestBcryptMatcher_Integration(t *testing.T) {
	t.Parallel()

	const (
		validID       = "key-active-1"
		validSecret   = "valid-secret-plaintext-abcdef"
		inactiveID    = "key-inactive-1"
		inactiveSec   = "inactive-secret-plaintext-xyz"
		wrongPlainKey = "wrong-secret-plaintext-nope"
	)

	loader := &bcryptLoader{}
	loader.add(validID, validSecret, true)
	loader.add(inactiveID, inactiveSec, false)

	// Match func mirroring an api-authorizer caller: a bcrypt comparison of the
	// raw secret against the stored hash. All comparison cost lives here, keeping
	// the cache package itself dependency-free.
	match := func(raw string, k apiKey) bool {
		return bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(raw)) == nil
	}

	clock := &integrationClock{current: time.Now()}
	c := cache.New(loader, match, cache.WithNowFunc(clock.Now))
	ctx := context.Background()

	// A valid plaintext resolves to the matching record.
	result, err := c.Validate(ctx, validSecret)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, validID, result.ID)

	// A wrong plaintext resolves to not-found.
	result, err = c.Validate(ctx, wrongPlainKey)
	require.NoError(t, err)
	assert.Nil(t, result, "wrong secret should not match")

	// An inactive key's plaintext resolves to not-found (loader omits it).
	result, err = c.Validate(ctx, inactiveSec)
	require.NoError(t, err)
	assert.Nil(t, result, "inactive key should not match")

	// A repeated valid lookup is served from cache without re-invoking the loader,
	// proving the seam caches across the bcrypt boundary.
	countBefore := loader.getCallCount()
	result, err = c.Validate(ctx, validSecret)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, validID, result.ID)
	assert.Equal(t, countBefore, loader.getCallCount(), "cache hit should not re-invoke the loader")
}
