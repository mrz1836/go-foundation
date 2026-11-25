package secrets_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/secrets"
)

// testSecret is a fake secret whose appearance in any output is a leak.
const testSecret = "tok-9f2c41d7-never-print-me" //nolint:gosec // G101: a fake value the tests search for

// errWrapped is the sentinel the wrapped-error tests wrap.
var errWrapped = errors.New("request failed")

// holder nests a Value in every shape a caller might put one in, exported and
// unexported.
type holder struct {
	Exported   secrets.Value
	Pointer    *secrets.Value
	Slice      []secrets.Value
	Map        map[string]secrets.Value
	unexported secrets.Value
	unexpMap   map[string]secrets.Value
	unexpSlice []secrets.Value
}

// newHolder returns a holder with testSecret in every field.
func newHolder() holder {
	v := secrets.NewValue(testSecret)

	return holder{
		Exported:   v,
		Pointer:    &v,
		Slice:      []secrets.Value{v},
		Map:        map[string]secrets.Value{"k": v},
		unexported: v,
		unexpMap:   map[string]secrets.Value{"k": v},
		unexpSlice: []secrets.Value{v},
	}
}

func TestValueRevealReturnsTheSecret(t *testing.T) {
	v := secrets.NewValue(testSecret)

	assert.Equal(t, testSecret, v.Reveal())
	assert.Equal(t, []byte(testSecret), v.Bytes())
	assert.False(t, v.IsZero())
}

func TestValueZeroIsEmpty(t *testing.T) {
	var v secrets.Value

	assert.True(t, v.IsZero())
	assert.Empty(t, v.Reveal())
	assert.Empty(t, v.Bytes())
	assert.True(t, secrets.NewValue("").IsZero())
	assert.Equal(t, secrets.Redacted, v.String())
}

func TestValueFromBytesCopiesTheInput(t *testing.T) {
	b := []byte(testSecret)
	v := secrets.ValueFromBytes(b)

	for i := range b {
		b[i] = 0
	}

	assert.Equal(t, testSecret, v.Reveal())
}

func TestValueBytesReturnsACopy(t *testing.T) {
	v := secrets.NewValue(testSecret)

	b := v.Bytes()
	b[0] = 'X'

	assert.Equal(t, testSecret, v.Reveal())
}

func TestValueEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b secrets.Value
		want bool
	}{
		{name: "same secret", a: secrets.NewValue(testSecret), b: secrets.NewValue(testSecret), want: true},
		{name: "different secret", a: secrets.NewValue(testSecret), b: secrets.NewValue("other"), want: false},
		{name: "prefix only", a: secrets.NewValue(testSecret), b: secrets.NewValue(testSecret[:4]), want: false},
		{name: "both zero", a: secrets.Value{}, b: secrets.NewValue(""), want: true},
		{name: "zero and set", a: secrets.Value{}, b: secrets.NewValue(testSecret), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.Equal(tt.b))
			assert.Equal(t, tt.want, tt.b.Equal(tt.a))
		})
	}
}

func TestValueFormatRedactsEveryVerb(t *testing.T) {
	v := secrets.NewValue(testSecret)
	verbs := []string{
		"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%t",
		"%10s", "%-10v", "%.3s", "% x", "%#q", "%U", "%c", "%e", "%b", "%o",
	}

	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			assert.Equal(t, secrets.Redacted, fmt.Sprintf(verb, v))
			assert.Equal(t, secrets.Redacted, fmt.Sprintf(verb, &v))
		})
	}

	// fmt handles %p itself, before Format: it prints an address, never the
	// secret.
	assert.NotContains(t, fmt.Sprintf("%p", v), testSecret)
	assert.NotContains(t, fmt.Sprintf("%p", &v), testSecret)

	assert.Equal(t, secrets.Redacted, fmt.Sprint(v))
	assert.Equal(t, secrets.Redacted+"\n", fmt.Sprintln(v))
	assert.Equal(t, secrets.Redacted, v.GoString())
}

func TestValueFormatNilPointerDoesNotPanic(t *testing.T) {
	var v *secrets.Value

	assert.NotPanics(t, func() {
		assert.NotContains(t, fmt.Sprintf("%v", v), testSecret)
	})
}

func TestValueNestedFormatNeverLeaks(t *testing.T) {
	h := newHolder()

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
		t.Run(verb, func(t *testing.T) {
			assert.NotContains(t, fmt.Sprintf(verb, h), testSecret)
			assert.NotContains(t, fmt.Sprintf(verb, &h), testSecret)
			assert.NotContains(t, fmt.Sprintf(verb, []holder{h}), testSecret)
			assert.NotContains(t, fmt.Sprintf(verb, map[string]holder{"h": h}), testSecret)
		})
	}
}

func TestValueWrappedErrorsNeverLeak(t *testing.T) {
	v := secrets.NewValue(testSecret)
	h := newHolder()

	errs := []error{
		fmt.Errorf("calling with key %v: %w", v, errWrapped),
		fmt.Errorf("outer: %w", fmt.Errorf("inner key=%s: %w", v, errWrapped)),
		fmt.Errorf("config %+v: %w", h, errWrapped),
		errors.Join(fmt.Errorf("key %q: %w", v, errWrapped), errWrapped),
	}

	for i, err := range errs {
		assert.NotContainsf(t, err.Error(), testSecret, "error %d", i)
		assert.NotContainsf(t, fmt.Sprintf("%+v", err), testSecret, "error %d", i)
		require.ErrorIs(t, err, errWrapped)
	}
}

func TestValueSlogRedacts(t *testing.T) {
	v := secrets.NewValue(testSecret)
	h := newHolder()

	handlers := map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
	}

	for name, newHandler := range handlers {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(newHandler(&buf))

			logger.Info("calling provider",
				slog.Any("key", v),
				slog.Any("pointer", &v),
				slog.Any("holder", h),
				slog.Group("group", slog.Any("key", v)),
				"loose", v,
			)
			logger.With("key", v).Info("with attrs")

			out := buf.String()
			assert.NotContains(t, out, testSecret)
			assert.Contains(t, out, secrets.Redacted)
		})
	}
}

func TestValueMarshalRedacts(t *testing.T) {
	v := secrets.NewValue(testSecret)
	h := newHolder()

	t.Run("json", func(t *testing.T) {
		for _, in := range []any{v, &v, h, []secrets.Value{v}, map[string]secrets.Value{"k": v}} {
			out, err := json.Marshal(in)
			require.NoError(t, err)
			assert.NotContains(t, string(out), testSecret)
			assert.Contains(t, string(out), secrets.Redacted)
		}
	})

	t.Run("json value", func(t *testing.T) {
		out, err := json.Marshal(v)
		require.NoError(t, err)
		assert.JSONEq(t, `"[REDACTED]"`, string(out))
	})

	t.Run("text", func(t *testing.T) {
		out, err := v.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, secrets.Redacted, string(out))
	})

	t.Run("xml", func(t *testing.T) {
		type doc struct {
			XMLName xml.Name        `xml:"doc"`
			Attr    secrets.Value   `xml:"attr,attr"`
			Elem    secrets.Value   `xml:"elem"`
			Items   []secrets.Value `xml:"item"`
		}

		out, err := xml.Marshal(doc{Attr: v, Elem: v, Items: []secrets.Value{v}})
		require.NoError(t, err)
		assert.NotContains(t, string(out), testSecret)
		assert.Equal(t, 3, strings.Count(string(out), secrets.Redacted))
	})
}

// BenchmarkValueEqual measures the constant-time comparison of two 64-byte
// secrets.
func BenchmarkValueEqual(b *testing.B) {
	a := secrets.NewValue(strings.Repeat("a", 64))
	c := secrets.NewValue(strings.Repeat("a", 64))

	b.ReportAllocs()

	for range b.N {
		_ = a.Equal(c)
	}
}
