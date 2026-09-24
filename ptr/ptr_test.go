package ptr_test

import (
	"testing"

	"github.com/mrz1836/go-foundation/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTo(t *testing.T) {
	t.Parallel()

	p := ptr.To(42)
	require.NotNil(t, p)
	assert.Equal(t, 42, *p)

	s := ptr.To("hello")
	require.NotNil(t, s)
	assert.Equal(t, "hello", *s)
}

func TestDeref(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 7, ptr.Deref(ptr.To(7)))
	assert.Equal(t, 0, ptr.Deref[int](nil))
	assert.Empty(t, ptr.Deref[string](nil))
}

func TestDerefOr(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 7, ptr.DerefOr(ptr.To(7), 99))
	assert.Equal(t, 99, ptr.DerefOr[int](nil, 99))
	assert.Equal(t, "def", ptr.DerefOr[string](nil, "def"))
}

// BenchmarkTo measures the pointer-boxing helper. Because the result is
// discarded it does not escape and stays stack-allocated (0 allocs); ReportAllocs
// guards against a regression that would force the value to the heap.
func BenchmarkTo(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_ = ptr.To(42)
	}
}

// BenchmarkDeref measures the nil-safe dereference on the non-nil path.
func BenchmarkDeref(b *testing.B) {
	p := ptr.To(42)

	b.ReportAllocs()

	for range b.N {
		_ = ptr.Deref(p)
	}
}
