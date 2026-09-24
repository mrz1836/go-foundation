package strtmpl_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mrz1836/go-foundation/strtmpl"
	"github.com/stretchr/testify/assert"
)

func TestRenderBasic(t *testing.T) {
	t.Parallel()

	out, unknown := strtmpl.Render("hello {{name}}", map[string]string{"name": "world"})
	assert.Equal(t, "hello world", out)
	assert.Empty(t, unknown)
}

func TestRenderInnerSpaces(t *testing.T) {
	t.Parallel()

	out, unknown := strtmpl.Render("{{ name }} and {{name}}", map[string]string{"name": "x"})
	assert.Equal(t, "x and x", out)
	assert.Empty(t, unknown)
}

func TestRenderFirstSourceWins(t *testing.T) {
	t.Parallel()

	public := map[string]string{"token": "PUBLIC"}
	secret := map[string]string{"token": "SECRET", "key": "K"}

	out, unknown := strtmpl.Render("{{token}}-{{key}}", public, secret)
	assert.Equal(t, "PUBLIC-K", out)
	assert.Empty(t, unknown)
}

// TestRenderSubstitutedValueNotRescanned is the injection-safety property: a
// value substituted for one placeholder is never itself scanned for further
// placeholders, so a low-trust value shaped like "{{secret}}" cannot resolve
// against a later source.
func TestRenderSubstitutedValueNotRescanned(t *testing.T) {
	t.Parallel()

	public := map[string]string{"user_input": "{{secret}}"}
	secret := map[string]string{"secret": "TOP_SECRET"}

	out, unknown := strtmpl.Render("{{user_input}}", public, secret)
	assert.Equal(t, "{{secret}}", out)
	assert.Empty(t, unknown)
}

func TestRenderUnknownPlaceholders(t *testing.T) {
	t.Parallel()

	out, unknown := strtmpl.Render("{{a}}-{{b}}-{{a}}", map[string]string{"a": "1"})
	assert.Equal(t, "1--1", out)
	assert.Equal(t, []string{"b"}, unknown)
}

func TestRenderReportsEachUnresolvedOccurrence(t *testing.T) {
	t.Parallel()

	out, unknown := strtmpl.Render("{{x}} {{x}}")
	assert.Equal(t, " ", out)
	assert.Equal(t, []string{"x", "x"}, unknown)
}

func TestRenderNoPlaceholders(t *testing.T) {
	t.Parallel()

	out, unknown := strtmpl.Render("plain text")
	assert.Equal(t, "plain text", out)
	assert.Empty(t, unknown)
}

// BenchmarkRender sweeps templates with an increasing number of placeholders so
// the regexp scan plus per-placeholder map lookups are measured as work grows.
func BenchmarkRender(b *testing.B) {
	vars := map[string]string{"a": "1", "b": "2", "c": "3"}

	for _, reps := range []int{1, 8, 64} {
		var sb strings.Builder
		for range reps {
			sb.WriteString("x {{a}} {{b}} {{c}} ")
		}
		tmpl := sb.String()

		b.Run(fmt.Sprintf("placeholders=%d", reps*3), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_, _ = strtmpl.Render(tmpl, vars)
			}
		})
	}
}

// FuzzRender asserts Render never panics on arbitrary template input: any
// malformed or partial placeholder syntax must render (possibly to itself),
// never crash.
func FuzzRender(f *testing.F) {
	for _, seed := range []string{"", "plain", "{{a}}", "{{ a }}", "{{a}}{{b}}", "{{unclosed", "}}{{", "{{a}} literal {{b}}"} {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, tmpl string) {
		_, _ = strtmpl.Render(tmpl, map[string]string{"a": "{{b}}", "b": "SECRET"})
	})
}
