package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
)

// baseTestID is a typed string ID used to instantiate the generic BaseModel.
type baseTestID string

func TestBaseModel_GetID(t *testing.T) {
	t.Parallel()

	bm := models.BaseModel[baseTestID]{ID: "abc-123"}
	assert.Equal(t, "abc-123", bm.GetID(), "GetID must return the ID as a plain string")
}

func TestBaseModel_IsDeleted(t *testing.T) {
	t.Parallel()

	t.Run("false when DeletedAt is unset", func(t *testing.T) {
		t.Parallel()

		bm := models.BaseModel[baseTestID]{}
		assert.False(t, bm.IsDeleted())
	})

	t.Run("true when DeletedAt is valid", func(t *testing.T) {
		t.Parallel()

		bm := models.BaseModel[baseTestID]{
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		}
		assert.True(t, bm.IsDeleted())
	})
}
