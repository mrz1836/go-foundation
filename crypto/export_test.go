package crypto

import "io"

// SetRandReader swaps the package CSPRNG source and returns a function that
// restores the previous source. It exists so the black-box tests in
// package crypto_test can drive a failing reader through RandomBytes and
// HashPasswordWithParams to cover the otherwise-unreachable CSPRNG error paths.
//
// Callers must restore the reader before returning; tests using this seam must
// not call t.Parallel(), since randReader is process-global.
func SetRandReader(r io.Reader) func() {
	prev := randReader
	randReader = r

	return func() { randReader = prev }
}
