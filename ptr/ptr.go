// Package ptr provides tiny generic helpers for working with pointers, so
// callers stop writing one-off "address of a literal" locals or nil-checking
// deref blocks. It is dependency-free.
package ptr

// To returns a pointer to v. It is the idiomatic way to take the address of a
// literal or a function result (for example an optional struct field or a
// pointer-typed API argument) without an intermediate local variable.
func To[T any](v T) *T {
	return &v
}

// Deref returns the value p points to, or the zero value of T when p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}

	return *p
}

// DerefOr returns the value p points to, or def when p is nil.
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}

	return *p
}
