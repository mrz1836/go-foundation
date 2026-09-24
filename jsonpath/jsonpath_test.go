package jsonpath_test

import (
	"encoding/json"
	"testing"

	"github.com/mrz1836/go-foundation/jsonpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, raw string) any {
	t.Helper()

	var v any
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	return v
}

func TestEval(t *testing.T) {
	t.Parallel()

	tree := decode(t, `{"a":{"b":"x"},"list":[{"id":1},{"id":2}],"n":42,"nested":{"deep key":"v"}}`)

	tests := []struct {
		name   string
		path   string
		want   any
		wantOK bool
	}{
		{"dotted", "$.a.b", "x", true},
		{"numeric index", "$.list[0].id", float64(1), true},
		{"second index", "$.list[1].id", float64(2), true},
		{"bracket single quote", "$['a']['b']", "x", true},
		{"bracket double quote", `$["a"]["b"]`, "x", true},
		{"scalar", "$.n", float64(42), true},
		{"missing key", "$.a.c", nil, false},
		{"index out of range", "$.list[5]", nil, false},
		{"negative index", "$.list[-1]", nil, false},
		{"index into non-array", "$.a[0]", nil, false},
		{"member of non-object", "$.n.x", nil, false},
		{"bracketed key with space", "$.nested['deep key']", "v", true},
		{"bad syntax no dot", "a.b", nil, false},
		{"unterminated bracket", "$.list[0", nil, false},
		{"root only", "$", tree, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := jsonpath.Eval(tree, tt.path)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// BenchmarkEval sweeps representative path shapes over a fixed tree so a
// regression in the path parser or the tree walk surfaces per shape.
func BenchmarkEval(b *testing.B) {
	var tree any
	if err := json.Unmarshal([]byte(`{"a":{"b":{"c":"x"}},"list":[{"id":1},{"id":2},{"id":3}]}`), &tree); err != nil {
		b.Fatal(err)
	}

	for _, path := range []string{"$.a.b.c", "$.list[2].id", "$['a']['b']['c']"} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_, _ = jsonpath.Eval(tree, path)
			}
		})
	}
}

// FuzzEval asserts Eval never panics on arbitrary path input over a fixed tree:
// malformed syntax must be rejected as (nil, false), never a crash.
func FuzzEval(f *testing.F) {
	var tree any
	if err := json.Unmarshal([]byte(`{"a":{"b":"x"},"list":[{"id":1}]}`), &tree); err != nil {
		f.Fatal(err)
	}

	for _, seed := range []string{"$.a.b", "$.list[0].id", "$['a']", "", "$", "$.", "$[", "$[999]", "not-a-path"} {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, path string) {
		_, _ = jsonpath.Eval(tree, path)
	})
}
