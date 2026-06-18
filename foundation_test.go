package foundation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation"
)

// TestModulePath verifies the canonical module path constant.
func TestModulePath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "github.com/mrz1836/go-foundation", foundation.ModulePath)
}
