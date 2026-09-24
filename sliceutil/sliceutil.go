// Package sliceutil provides small, dependency-free generic slice helpers that
// the standard library's slices package does not cover — notably order-preserving
// deduplication of an unsorted slice and set intersection. They exist so services
// stop re-deriving the same map-backed loops locally.
package sliceutil

// Dedupe returns a new slice with duplicate elements removed, preserving the
// first-seen order of the input. It returns nil for a nil or empty input. Unlike
// slices.Compact it does not require the input to be sorted, so it is safe for an
// arbitrarily-ordered slice where order must be preserved.
func Dedupe[T comparable](in []T) []T {
	if len(in) == 0 {
		return nil
	}

	seen := make(map[T]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	return out
}

// Intersect returns the elements of a that are also present in b, preserving a's
// order. b is treated as a membership set, so its order and duplicates are
// irrelevant; a's duplicates are preserved (dedupe a first if that is not
// wanted). It returns nil when a is empty.
func Intersect[T comparable](a, b []T) []T {
	if len(a) == 0 {
		return nil
	}

	set := make(map[T]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}

	out := make([]T, 0, len(a))
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}

	return out
}
