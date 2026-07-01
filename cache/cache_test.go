package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test fixtures: a generic item, a mock loader, and an equality match function
// ============================================================================

// testItem is a minimal record used to exercise the generic cache mechanics.
// The equality match function compares against its secret field, so the tests
// never need a real hashing dependency.
type testItem struct {
	id     string
	secret string
}

// matchEqual reports whether the raw secret equals the item's secret. It stands
// in for a hash comparison (for example bcrypt) so the white-box tests can drive
// every cache branch deterministically without any external dependency.
func matchEqual(raw string, it testItem) bool {
	return raw == it.secret
}

// mockLoader implements Loader[testItem] for testing. It mirrors the semantics
// of a repository's "find all active" query: only items added as active are ever
// returned, an injectable error can force LoadAll to fail, and an atomic counter
// records how many times LoadAll was invoked.
type mockLoader struct {
	mu        sync.RWMutex
	items     []testItem
	callCount int64 // atomic counter for tracking loads
	errOnCall error // if set, LoadAll returns this error
}

func newMockLoader() *mockLoader {
	return &mockLoader{items: make([]testItem, 0)}
}

// LoadAll returns a copy of the loaded items (or the injected error). It bumps
// an atomic call counter so tests can assert how often the slow path ran.
func (m *mockLoader) LoadAll(_ context.Context) ([]testItem, error) {
	atomic.AddInt64(&m.callCount, 1)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.errOnCall != nil {
		return nil, m.errOnCall
	}

	result := make([]testItem, len(m.items))
	copy(result, m.items)
	return result, nil
}

// addKey records a secret. Inactive keys are never stored, so the loader only
// ever returns active items — mirroring a repository's active-only filtering.
func (m *mockLoader) addKey(secret string, active bool) {
	if !active {
		return
	}

	m.mu.Lock()
	m.items = append(m.items, testItem{id: secret, secret: secret})
	m.mu.Unlock()
}

func (m *mockLoader) setError(err error) {
	m.mu.Lock()
	m.errOnCall = err
	m.mu.Unlock()
}

func (m *mockLoader) getCallCount() int64 {
	return atomic.LoadInt64(&m.callCount)
}

func (m *mockLoader) resetCallCount() {
	atomic.StoreInt64(&m.callCount, 0)
}

// ============================================================================
// Test Time Controller
// ============================================================================

// testClock provides a controllable clock for testing.
type testClock struct {
	mu      sync.RWMutex
	current time.Time
}

func newTestClock(start time.Time) *testClock {
	return &testClock{current: start}
}

func (c *testClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(d)
}

func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = t
}

// ============================================================================
// Constructor / option Tests
// ============================================================================

func TestNew_DefaultTTL(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	c := New(loader, matchEqual)

	assert.Equal(t, DefaultTTL, c.ttl)
	assert.Equal(t, DefaultInvalidTTL, c.invalidTTL)
	assert.Equal(t, DefaultMaxEntries, c.maxEntries)
	assert.NotNil(t, c.entries)
	assert.NotNil(t, c.now)
}

func TestNew_WithTTL(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	customTTL := 30 * time.Second
	c := New(loader, matchEqual, WithTTL(customTTL))

	assert.Equal(t, customTTL, c.ttl)
}

func TestNew_WithInvalidTTL(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	customInvalidTTL := 15 * time.Second
	c := New(loader, matchEqual, WithInvalidTTL(customInvalidTTL))

	assert.Equal(t, customInvalidTTL, c.invalidTTL)
}

func TestNew_WithMaxEntries(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	c := New(loader, matchEqual, WithMaxEntries(500))

	assert.Equal(t, 500, c.maxEntries)
}

func TestNew_WithNowFunc(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	clock := newTestClock(time.Now())
	c := New(loader, matchEqual, WithNowFunc(clock.Now))

	// Verify the custom now function is used.
	expected := clock.Now()
	actual := c.now()
	assert.Equal(t, expected, actual)
}

// ============================================================================
// Validation Tests
// ============================================================================

func TestCache_ValidKey_FirstCall(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "valid-key-fixture"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	result, err := c.Validate(ctx, secret)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, secret, result.secret)
	assert.Equal(t, secret, result.id)
	assert.Equal(t, int64(1), loader.getCallCount(), "Should load on first call")
}

func TestCache_ValidKey_CacheHit(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_cachedhit1234567890123"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// First call - populates cache.
	_, err := c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Second call - should use cache.
	result, err := c.Validate(ctx, secret)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(1), loader.getCallCount(), "Should NOT load on cache hit")
}

func TestCache_InvalidKey_Cached(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	loader.addKey("test_realkey123456789012345", true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	invalidKey := "test_fakekey123456789012345"

	// First call - should load.
	result, err := c.Validate(ctx, invalidKey)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Second call - should use cached negative result.
	result, err = c.Validate(ctx, invalidKey)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, int64(1), loader.getCallCount(), "Invalid key should be cached")
}

func TestCache_TTLExpiration_ValidKey(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_ttlexpire12345678901234"
	loader.addKey(secret, true)

	clock := newTestClock(time.Now())
	c := New(
		loader,
		matchEqual,
		WithTTL(60*time.Second),
		WithNowFunc(clock.Now),
	)
	ctx := context.Background()

	// First call.
	_, err := c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Second call (still fresh).
	_, err = c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Advance time past TTL.
	clock.Advance(61 * time.Second)

	// Third call (should refresh).
	_, err = c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(2), loader.getCallCount(), "Should refresh after TTL expiration")
}

func TestCache_TTLExpiration_InvalidKey(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	loader.addKey("test_realkey999999999999999", true)

	clock := newTestClock(time.Now())
	c := New(
		loader,
		matchEqual,
		WithTTL(60*time.Second),
		WithInvalidTTL(5*time.Second),
		WithNowFunc(clock.Now),
	)
	ctx := context.Background()

	invalidKey := "invalid-ttl-fixture"

	// First call.
	_, err := c.Validate(ctx, invalidKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Advance time past the invalid TTL (5s) but not past the loaded TTL (60s).
	clock.Advance(6 * time.Second)

	// Second call - the invalid entry has expired, but the loaded set is still
	// fresh, so the entry is rechecked against the loaded set without reloading.
	_, err = c.Validate(ctx, invalidKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount(), "Loaded set still fresh - no reload")
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_invalidateall123456789"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Populate cache.
	_, err := c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount())

	// Invalidate all.
	c.Invalidate()

	// Next call should reload.
	_, err = c.Validate(ctx, secret)
	require.NoError(t, err)
	assert.Equal(t, int64(2), loader.getCallCount(), "Should refresh after invalidation")
}

func TestCache_InvalidateKey(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret1 := "cache-key-one"
	secret2 := "cache-key-two"
	loader.addKey(secret1, true)
	loader.addKey(secret2, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Populate cache with both keys.
	_, err := c.Validate(ctx, secret1)
	require.NoError(t, err)
	_, err = c.Validate(ctx, secret2)
	require.NoError(t, err)
	assert.Equal(t, int64(1), loader.getCallCount()) // Only 1 load for both.

	// Invalidate only key1.
	c.InvalidateKey(secret1)

	loader.resetCallCount()

	// Key2 should still be cached.
	_, err = c.Validate(ctx, secret2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), loader.getCallCount(), "Key2 should still be cached")

	// Key1 needs revalidation, but the loaded set is still valid so no reload.
	_, err = c.Validate(ctx, secret1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), loader.getCallCount(), "Loaded set still valid - no reload")
}

func TestCache_Stats(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	loader.addKey("test_statskey1234567890123", true)

	clock := newTestClock(time.Now())
	c := New(loader, matchEqual, WithNowFunc(clock.Now))
	ctx := context.Background()

	// Initial stats.
	stats := c.Stats()
	assert.Equal(t, 0, stats.EntryCount)
	assert.Equal(t, 0, stats.LoadedCount)
	assert.Equal(t, time.Duration(0), stats.LoadedAge)

	// Add some entries.
	_, _ = c.Validate(ctx, "test_statskey1234567890123")
	_, _ = c.Validate(ctx, "test_invalidkey12345678901") // Invalid.

	stats = c.Stats()
	assert.Equal(t, 2, stats.EntryCount)
	assert.Equal(t, 1, stats.LoadedCount)
	assert.Equal(t, 1, stats.ValidEntries)
	assert.Equal(t, 1, stats.InvalidEntries)
}

func TestCache_LoaderError(t *testing.T) {
	t.Parallel()

	loaderErr := errors.New("loader connection failed")

	loader := newMockLoader()
	loader.setError(loaderErr)

	c := New(loader, matchEqual)
	ctx := context.Background()

	result, err := c.Validate(ctx, "test_anykey12345678901234567")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, loaderErr)
}

func TestCache_InactiveKey_NotReturned(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_inactivekey123456789012"
	loader.addKey(secret, false) // Inactive - loader omits it.

	c := New(loader, matchEqual)
	ctx := context.Background()

	result, err := c.Validate(ctx, secret)

	require.NoError(t, err)
	assert.Nil(t, result, "Inactive key should not be validated")
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestCache_ConcurrentReads(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_concurrent123456789012"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Pre-populate cache.
	_, _ = c.Validate(ctx, secret)
	loader.resetCallCount()

	// Concurrent reads.
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := c.Validate(ctx, secret)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		}()
	}

	wg.Wait()

	// Should not have made any loads (all cache hits).
	assert.Equal(t, int64(0), loader.getCallCount(), "Concurrent reads should all be cache hits")
}

func TestCache_ConcurrentColdStart(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()
	secret := "test_coldstart123456789012"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Concurrent cold start (all goroutines hit an empty cache).
	var wg sync.WaitGroup
	numGoroutines := 50
	results := make(chan *testItem, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := c.Validate(ctx, secret)
			assert.NoError(t, err)
			results <- result
		}()
	}

	wg.Wait()
	close(results)

	// All should get valid results.
	for result := range results {
		assert.NotNil(t, result)
	}

	// Double-check locking should minimize loads (might be more than 1 due to a
	// race at the very start, but should be small).
	assert.LessOrEqual(t, loader.getCallCount(), int64(5), "Double-check locking should minimize loads")
}

func TestCache_ConcurrentDifferentKeys(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()

	// Add multiple keys.
	keys := []string{
		"test_multikey1_123456789012",
		"test_multikey2_123456789012",
		"test_multikey3_123456789012",
		"test_multikey4_123456789012",
		"test_multikey5_123456789012",
	}
	for _, k := range keys {
		loader.addKey(k, true)
	}

	c := New(loader, matchEqual)
	ctx := context.Background()

	var wg sync.WaitGroup

	// Each key validated by multiple goroutines.
	for _, key := range keys {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				result, err := c.Validate(ctx, k)
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}(key)
		}
	}

	wg.Wait()

	// Verify all keys are cached.
	stats := c.Stats()
	assert.Equal(t, len(keys), stats.ValidEntries)
}

// ============================================================================
// Cache Entry Expiration Tests
// ============================================================================

func TestEntry_IsExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		checkedAt time.Time
		ttl       time.Duration
		now       time.Time
		expected  bool
	}{
		{
			name:      "not expired - just created",
			checkedAt: time.Now(),
			ttl:       60 * time.Second,
			now:       time.Now(),
			expected:  false,
		},
		{
			name:      "not expired - 30s old with 60s TTL",
			checkedAt: time.Now().Add(-30 * time.Second),
			ttl:       60 * time.Second,
			now:       time.Now(),
			expected:  false,
		},
		{
			name:      "expired - 61s old with 60s TTL",
			checkedAt: time.Now().Add(-61 * time.Second),
			ttl:       60 * time.Second,
			now:       time.Now(),
			expected:  true,
		},
		{
			name:      "expired - exactly at TTL",
			checkedAt: time.Now().Add(-60 * time.Second),
			ttl:       60 * time.Second,
			now:       time.Now(),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := &entry[testItem]{
				checkedAt: tt.checkedAt,
				ttl:       tt.ttl,
			}
			assert.Equal(t, tt.expected, e.isExpired(tt.now))
		})
	}
}

// ============================================================================
// Eviction Tests
// ============================================================================

func TestCache_MaxEntriesEviction(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()

	// Add a valid key for the cache to load.
	loader.addKey("test_evicttest123456789012", true)

	clock := newTestClock(time.Now())
	maxEntries := 100
	c := New(loader, matchEqual, WithNowFunc(clock.Now), WithMaxEntries(maxEntries))
	ctx := context.Background()

	// Fill cache with unique entries (manually, to test eviction).
	c.mu.Lock()
	for i := 0; i < maxEntries; i++ {
		key := fmt.Sprintf("test_fill_%05d_key", i)
		c.entries[key] = &entry[testItem]{
			valid:     false,
			checkedAt: clock.Now().Add(-time.Duration(i) * time.Second),
			ttl:       DefaultInvalidTTL,
		}
	}
	c.mu.Unlock()

	initialCount := len(c.entries)
	assert.Equal(t, maxEntries, initialCount)

	// Add one more entry - should trigger eviction.
	_, _ = c.Validate(ctx, "test_triggerevict12345678")

	// Should have evicted some entries.
	assert.Less(t, len(c.entries), maxEntries)
}

func TestCache_MaxEntriesEviction_RemovesOldestWhenNoneExpired(t *testing.T) {
	t.Parallel()

	loader := newMockLoader()

	// Add a valid key for the cache to load.
	loader.addKey("test_evictoldest123456789012", true)

	clock := newTestClock(time.Now())
	maxEntries := 100
	c := New(loader, matchEqual, WithNowFunc(clock.Now), WithMaxEntries(maxEntries))
	ctx := context.Background()

	// Fill the cache to capacity with entries that are NOT expired (long TTL), so
	// removeExpiredEntries deletes nothing and the oldest-entries eviction path in
	// evictOldestLocked must run.
	c.mu.Lock()
	for i := 0; i < maxEntries; i++ {
		key := fmt.Sprintf("test_oldest_%05d_key", i)
		c.entries[key] = &entry[testItem]{
			valid:     false,
			checkedAt: clock.Now().Add(-time.Duration(i) * time.Second),
			ttl:       time.Hour, // Long enough that none of these ages expire.
		}
	}
	c.mu.Unlock()

	assert.Equal(t, maxEntries, len(c.entries))

	// Add one more entry - should trigger the oldest-entries eviction branch.
	_, _ = c.Validate(ctx, "test_triggeroldest12345678")

	// The oldest ~10% should have been evicted before the new entry was added.
	assert.Less(t, len(c.entries), maxEntries)
}

func TestCache_RemoveOldestEntries_DynamicPercentage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entryCount     int
		expectedRemove int // minimum entries to remove
	}{
		{
			name:           "fewer than 10 entries removes at least 1",
			entryCount:     5,
			expectedRemove: 1,
		},
		{
			name:           "exactly 10 entries removes 1",
			entryCount:     10,
			expectedRemove: 1,
		},
		{
			name:           "100 entries removes 10",
			entryCount:     100,
			expectedRemove: 10,
		},
		{
			name:           "1000 entries removes 100",
			entryCount:     1000,
			expectedRemove: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loader := newMockLoader()
			clock := newTestClock(time.Now())
			c := New(loader, matchEqual, WithNowFunc(clock.Now))

			// Fill cache with entries of varying ages.
			c.mu.Lock()
			for i := 0; i < tt.entryCount; i++ {
				key := fmt.Sprintf("test_dynamic_%05d_key", i)
				c.entries[key] = &entry[testItem]{
					valid:     false,
					checkedAt: clock.Now().Add(-time.Duration(i) * time.Second),
					ttl:       DefaultTTL, // Long TTL so entries don't expire.
				}
			}
			c.mu.Unlock()

			initialCount := len(c.entries)
			assert.Equal(t, tt.entryCount, initialCount)

			// Call removeOldestEntries directly.
			c.mu.Lock()
			c.removeOldestEntries(clock.Now())
			c.mu.Unlock()

			// Verify at least expectedRemove entries were removed.
			removed := initialCount - len(c.entries)
			assert.GreaterOrEqual(t, removed, tt.expectedRemove,
				"should remove at least %d entries, but only removed %d", tt.expectedRemove, removed)
		})
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkCache_CacheHit(b *testing.B) {
	loader := newMockLoader()
	secret := "test_benchmark123456789012"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Warm up cache.
	_, _ = c.Validate(ctx, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Validate(ctx, secret)
	}
}

func BenchmarkCache_CacheMiss(b *testing.B) {
	loader := newMockLoader()
	loader.addKey("test_realkey999999999999999", true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Each iteration uses a different invalid key to avoid caching.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test_miss" + string(rune('A'+i%26)) + "123456789"
		_, _ = c.Validate(ctx, key)
	}
}

func BenchmarkCache_ParallelReads(b *testing.B) {
	loader := newMockLoader()
	secret := "test_parallel123456789012"
	loader.addKey(secret, true)

	c := New(loader, matchEqual)
	ctx := context.Background()

	// Warm up cache.
	_, _ = c.Validate(ctx, secret)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Validate(ctx, secret)
		}
	})
}
