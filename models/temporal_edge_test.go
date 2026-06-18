package models_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrz1836/go-foundation/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sampleEdgeID is the typed ID used by the in-test edge model.
type sampleEdgeID string

// sampleEdge is a throwaway model embedding TemporalEdge, used to exercise
// the substrate against real SQLite.
type sampleEdge struct {
	models.TemporalEdge[sampleEdgeID]

	Label string
}

func newEdgeDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&sampleEdge{}))

	return db
}

func TestTemporalEdge_BeforeCreate_Defaults(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	edge := &sampleEdge{TemporalEdge: models.TemporalEdge[sampleEdgeID]{ValidFrom: validFrom}}
	require.NoError(t, db.Create(edge).Error)

	parsed, err := uuid.Parse(string(edge.ID))
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), parsed.Version())

	assert.WithinDuration(t, time.Now(), edge.RecordedAt, 5*time.Second)
	assert.False(t, edge.CreatedAt.IsZero())
	assert.Equal(t, "unverified", edge.VerificationStatus)

	assert.Nil(t, edge.ValidTo)
	assert.Nil(t, edge.EventTime)
	assert.Nil(t, edge.SupersededByID)
	assert.Nil(t, edge.SuppressedAt)
	assert.Nil(t, edge.Confidence)
}

func TestTemporalEdge_BeforeCreate_DoesNotDefaultValidFrom(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)

	edge := &sampleEdge{Label: "no-valid-from"}
	require.NoError(t, db.Create(edge).Error)

	assert.True(t, edge.ValidFrom.IsZero(), "ValidFrom must never be auto-defaulted")
}

func TestTemporalEdge_BeforeCreate_UsesContextClock(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)
	anchor := time.Date(2020, 6, 15, 8, 0, 0, 0, time.UTC)
	ctx := models.WithClock(context.Background(), models.NewFixedClock(anchor))

	edge := &sampleEdge{TemporalEdge: models.TemporalEdge[sampleEdgeID]{ValidFrom: anchor}}
	require.NoError(t, db.WithContext(ctx).Create(edge).Error)

	assert.Equal(t, anchor, edge.RecordedAt.UTC())
	assert.Equal(t, anchor, edge.CreatedAt.UTC())
}

func TestTemporalEdge_BeforeCreate_PreservesExplicitValues(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)

	id := sampleEdgeID(models.NewID())
	recordedAt := time.Date(2025, 3, 3, 3, 3, 3, 0, time.UTC)
	createdAt := time.Date(2024, 2, 2, 2, 2, 2, 0, time.UTC)

	edge := &sampleEdge{TemporalEdge: models.TemporalEdge[sampleEdgeID]{
		ID:                 id,
		ValidFrom:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		RecordedAt:         recordedAt,
		CreatedAt:          createdAt,
		VerificationStatus: "verified",
	}}
	require.NoError(t, db.Create(edge).Error)

	assert.Equal(t, id, edge.ID)
	assert.Equal(t, recordedAt, edge.RecordedAt.UTC())
	assert.Equal(t, createdAt, edge.CreatedAt.UTC())
	assert.Equal(t, "verified", edge.VerificationStatus)
}

func TestTemporalEdge_ActiveRowQuery(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)

	active := &sampleEdge{TemporalEdge: models.TemporalEdge[sampleEdgeID]{
		ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}}
	require.NoError(t, db.Create(active).Error)

	ended := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	closed := &sampleEdge{TemporalEdge: models.TemporalEdge[sampleEdgeID]{
		ValidFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidTo:   &ended,
	}}
	require.NoError(t, db.Create(closed).Error)

	var rows []sampleEdge
	require.NoError(t, db.Where("valid_to IS NULL").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, active.ID, rows[0].ID)
}

func TestTemporalEdge_AppendOnlySupersession(t *testing.T) {
	t.Parallel()

	db := newEdgeDB(t)

	original := &sampleEdge{
		TemporalEdge: models.TemporalEdge[sampleEdgeID]{ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		Label:        "pre-correction",
	}
	require.NoError(t, db.Create(original).Error)

	correction := &sampleEdge{
		TemporalEdge: models.TemporalEdge[sampleEdgeID]{ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		Label:        "corrected",
	}
	require.NoError(t, db.Create(correction).Error)

	suppressedAt := time.Now()
	require.NoError(t, db.Model(&sampleEdge{}).Where("id = ?", string(original.ID)).
		Updates(map[string]any{
			"superseded_by_id": string(correction.ID),
			"suppressed_at":    suppressedAt,
		}).Error)

	var all []sampleEdge
	require.NoError(t, db.Find(&all).Error)
	assert.Len(t, all, 2)

	var current []sampleEdge
	require.NoError(t, db.Where("suppressed_at IS NULL").Find(&current).Error)
	require.Len(t, current, 1)
	assert.Equal(t, correction.ID, current[0].ID)

	var asOf sampleEdge
	require.NoError(t, db.First(&asOf, "id = ?", string(original.ID)).Error)
	assert.Equal(t, "pre-correction", asOf.Label)
	require.NotNil(t, asOf.SupersededByID)
	assert.Equal(t, correction.ID, *asOf.SupersededByID)
	require.NotNil(t, asOf.SuppressedAt)
}

func TestTemporalEdge_DoesNotEmbedBaseModel(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(models.TemporalEdge[sampleEdgeID]{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		assert.NotContains(t, []string{"BaseModel", "DeletedAt", "Metadata"}, name,
			"TemporalEdge must not embed BaseModel or carry soft-delete/metadata fields")
	}
}
