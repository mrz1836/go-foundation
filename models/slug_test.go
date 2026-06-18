package models_test

import (
	"testing"

	"github.com/mrz1836/go-foundation/models"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "Hello World",
			want:  "hello-world",
		},
		{
			name:  "already lowercase",
			input: "hello world",
			want:  "hello-world",
		},
		{
			name:  "with underscores",
			input: "hello_world",
			want:  "hello-world",
		},
		{
			name:  "with special characters",
			input: "Hello! World@123",
			want:  "hello-world123",
		},
		{
			name:  "city with state",
			input: "Asheville NC",
			want:  "asheville-nc",
		},
		{
			name:  "multiple spaces",
			input: "hello   world",
			want:  "hello-world",
		},
		{
			name:  "leading and trailing spaces",
			input: "  hello world  ",
			want:  "hello-world",
		},
		{
			name:  "consecutive special chars",
			input: "hello---world",
			want:  "hello-world",
		},
		{
			name:  "numbers",
			input: "Route 66",
			want:  "route-66",
		},
		{
			name:  "unicode chars removed",
			input: "Café München",
			want:  "caf-mnchen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := models.GenerateSlug(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
