package models_test

import (
	"testing"
	"time"

	"github.com/mrz1836/go-foundation/models"
	"github.com/stretchr/testify/assert"
)

func TestExternalSource_IsExternal(t *testing.T) {
	t.Parallel()

	t.Run("nil provider", func(t *testing.T) {
		t.Parallel()

		es := models.ExternalSource{}
		assert.False(t, es.IsExternal())
	})

	t.Run("empty provider", func(t *testing.T) {
		t.Parallel()

		empty := ""
		es := models.ExternalSource{Provider: &empty}
		assert.False(t, es.IsExternal())
	})

	t.Run("non-empty provider", func(t *testing.T) {
		t.Parallel()

		provider := "leaguelinq"
		es := models.ExternalSource{Provider: &provider}
		assert.True(t, es.IsExternal())
	})
}

func TestValidateExternalSource(t *testing.T) {
	t.Parallel()

	t.Run("valid empty source", func(t *testing.T) {
		t.Parallel()

		es := &models.ExternalSource{}
		err := models.ValidateExternalSource(es)
		assert.NoError(t, err)
	})

	t.Run("valid with all fields", func(t *testing.T) {
		t.Parallel()

		provider := "leaguelinq"
		externalID := "ext-123"
		now := time.Now().UTC()
		es := &models.ExternalSource{
			Provider:   &provider,
			ExternalID: &externalID,
			LastSeenAt: &now,
		}
		err := models.ValidateExternalSource(es)
		assert.NoError(t, err)
	})
}
