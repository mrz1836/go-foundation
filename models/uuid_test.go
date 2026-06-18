package models_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// sampleEntityID is the typed ID used by the in-test BaseModel embedder.
type sampleEntityID string

// sampleEntity is a throwaway BaseModel-embedding type used to exercise the
// BeforeCreate ID-minting hook.
type sampleEntity struct {
	models.BaseModel[sampleEntityID]
}

// errRNGFailure simulates a process-level RNG fault.
var errRNGFailure = errors.New("rng failure")

// errReader always fails, simulating a process-level RNG fault.
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errRNGFailure
}

func TestNewID_ReturnsUUIDv7(t *testing.T) {
	t.Parallel()

	id := models.NewID()
	require.NotEmpty(t, id)

	parsed, err := uuid.Parse(id)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), parsed.Version())
}

func TestNewID_LexicographicOrdering(t *testing.T) {
	t.Parallel()

	const n = 1000

	ids := make([]string, n)
	for i := range ids {
		ids[i] = models.NewID()
	}

	for i := 1; i < n; i++ {
		assert.LessOrEqual(t, ids[i-1], ids[i],
			"v7 IDs must sort non-decreasingly in creation order")
	}
}

func TestNewID_PanicsOnRNGFailure(t *testing.T) {
	// Not parallel: mutates the package-global UUID rand source.
	uuid.SetRand(errReader{})
	t.Cleanup(func() { uuid.SetRand(nil) })

	assert.Panics(t, func() { _ = models.NewID() })
}

func TestBaseModel_BeforeCreate_MintsV7(t *testing.T) {
	t.Parallel()

	var e sampleEntity
	require.Empty(t, e.ID)

	require.NoError(t, e.BeforeCreate(nil))

	parsed, err := uuid.Parse(string(e.ID))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), parsed.Version())
}

func TestBaseModel_BeforeCreate_PreservesExplicitID(t *testing.T) {
	t.Parallel()

	explicit := sampleEntityID(models.NewID())
	e := sampleEntity{BaseModel: models.BaseModel[sampleEntityID]{ID: explicit}}

	require.NoError(t, e.BeforeCreate(nil))
	assert.Equal(t, explicit, e.ID)
}
