package sliceutil_test

import (
	"fmt"
	"testing"

	"github.com/mrz1836/go-foundation/sliceutil"
	"github.com/stretchr/testify/assert"
)

func TestDedupe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"no dups", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"preserves first-seen order", []string{"b", "a", "b", "c", "a"}, []string{"b", "a", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, sliceutil.Dedupe(tt.in))
		})
	}
}

func TestDedupeInts(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int{3, 1, 2}, sliceutil.Dedupe([]int{3, 1, 3, 2, 1}))
}

func TestIntersect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{"empty a", nil, []string{"a"}, nil},
		{"empty b keeps nothing", []string{"a", "b"}, nil, []string{}},
		{"keeps a order", []string{"c", "a", "b"}, []string{"a", "b", "c"}, []string{"c", "a", "b"}},
		{"filters to b membership", []string{"a", "b", "c"}, []string{"b"}, []string{"b"}},
		{"preserves a duplicates", []string{"a", "a", "b"}, []string{"a"}, []string{"a", "a"}},
		{"b duplicates irrelevant", []string{"a", "b"}, []string{"a", "a", "a"}, []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, sliceutil.Intersect(tt.a, tt.b))
		})
	}
}

// BenchmarkDedupe sweeps input sizes at a ~50% duplicate ratio so a regression
// in the map-backed first-seen scan surfaces as the slice grows.
func BenchmarkDedupe(b *testing.B) {
	for _, size := range []int{16, 256, 4096} {
		in := make([]int, size)
		for i := range in {
			in[i] = i % (size / 2)
		}

		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_ = sliceutil.Dedupe(in)
			}
		})
	}
}

// BenchmarkIntersect sweeps input sizes with a fully-overlapping filter (worst
// case: every element of a is retained) so the membership-set build plus the
// scan are measured at their most expensive.
func BenchmarkIntersect(b *testing.B) {
	for _, size := range []int{16, 256, 4096} {
		a := make([]int, size)
		filter := make([]int, size)
		for i := range a {
			a[i] = i
			filter[i] = i
		}

		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_ = sliceutil.Intersect(a, filter)
			}
		})
	}
}
