package config

import (
	"strings"
	"testing"
)

func FuzzEscapeDSNValue(f *testing.F) {
	f.Add("simple")
	f.Add("pass word")
	f.Add("pass'word")
	f.Add(`pass\word`)
	f.Add("p w's")
	f.Add(`p\'w`)
	f.Add("")
	f.Add("admin host=evil.com")

	f.Fuzz(func(t *testing.T, input string) {
		result := escapeDSNValue(input)

		// Must be wrapped in single quotes
		if !strings.HasPrefix(result, "'") || !strings.HasSuffix(result, "'") {
			t.Errorf("escapeDSNValue(%q) = %q, want single-quoted result", input, result)
		}

		// The inner content must not contain unescaped single quotes
		inner := result[1 : len(result)-1]
		assertNoUnescapedQuotes(t, input, inner)
	})
}

func assertNoUnescapedQuotes(t *testing.T, input, inner string) {
	t.Helper()

	for i := 0; i < len(inner); i++ {
		if inner[i] != '\'' {
			continue
		}

		if i+1 >= len(inner) || inner[i+1] != '\'' {
			t.Errorf("escapeDSNValue(%q) has unescaped single quote at position %d", input, i)
		}

		i++ // skip the escape pair
	}
}
