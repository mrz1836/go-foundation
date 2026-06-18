package models_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// ptr returns a pointer to v, for building optional ExternalSource fields.
func ptr[T any](v T) *T { return &v }

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

func TestValidateExternalSource_Rules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		source    *models.ExternalSource
		wantErr   bool
		wantField string // substring expected in the error message
	}{
		{
			name:   "nil source is valid",
			source: nil,
		},
		{
			name:   "empty source is valid",
			source: &models.ExternalSource{},
		},
		{
			name:   "external id only is valid",
			source: &models.ExternalSource{ExternalID: ptr("ext-123")},
		},
		{
			name:   "provider with external id is valid",
			source: &models.ExternalSource{Provider: ptr("leaguelinq"), ExternalID: ptr("ext-123")},
		},
		{
			name:      "provider without external id is rejected",
			source:    &models.ExternalSource{Provider: ptr("leaguelinq")},
			wantErr:   true,
			wantField: "external_id",
		},
		{
			name:      "provider with empty external id is rejected",
			source:    &models.ExternalSource{Provider: ptr("leaguelinq"), ExternalID: ptr("   ")},
			wantErr:   true,
			wantField: "external_id",
		},
		{
			name:   "whitespace-only provider is treated as unset",
			source: &models.ExternalSource{Provider: ptr("   ")},
		},
		{
			name:      "over-length provider is rejected",
			source:    &models.ExternalSource{Provider: ptr(strings.Repeat("p", 51)), ExternalID: ptr("ext-123")},
			wantErr:   true,
			wantField: "provider",
		},
		{
			name:      "over-length external id is rejected",
			source:    &models.ExternalSource{Provider: ptr("leaguelinq"), ExternalID: ptr(strings.Repeat("e", 501))},
			wantErr:   true,
			wantField: "external_id",
		},
		{
			name:   "boundary lengths are accepted",
			source: &models.ExternalSource{Provider: ptr(strings.Repeat("p", 50)), ExternalID: ptr(strings.Repeat("e", 500))},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := models.ValidateExternalSource(tc.source)
			if !tc.wantErr {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			require.ErrorIs(t, err, models.ErrValidation, "error must wrap ErrValidation")
			assert.Contains(t, err.Error(), tc.wantField, "error must reference the offending field")
		})
	}
}
