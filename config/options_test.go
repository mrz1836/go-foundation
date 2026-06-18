package config_test

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/config"
)

// newCapturingLogger returns a slog.Logger that writes structured text into buf,
// letting a test assert on exactly what the loader reported.
func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoadFromEnv_WithLoggerCapturesInvalidWarning(t *testing.T) {
	t.Setenv("L_I", "not-a-number")

	var buf bytes.Buffer
	s := scalars{I: 99}

	require.NoError(t, config.LoadFromEnv(&s, "L_", config.WithLogger(newCapturingLogger(&buf))))

	assert.Equal(t, 99, s.I, "lenient mode keeps the prior value on a parse failure")

	out := buf.String()
	assert.Contains(t, out, "ignoring invalid environment override")
	assert.Contains(t, out, "L_I")
	assert.Contains(t, out, "not-a-number")
}

func TestLoadFromEnv_WithLoggerNilIsIgnored(t *testing.T) {
	// A nil logger must not override the package default, and the loader must
	// still bind valid values without panicking.
	t.Setenv("LN_STR", "ok")

	s := scalars{}
	require.NoError(t, config.LoadFromEnv(&s, "LN_", config.WithLogger(nil)))
	assert.Equal(t, "ok", s.Str)
}

// withText pairs a value and pointer TextUnmarshaler so both binding paths see
// the same malformed input.
type withText struct {
	Num numText  `env:"NUM"`
	Ptr *numText `env:"PTR"`
}

func TestLoadFromEnv_TextUnmarshalerErrorLenient(t *testing.T) {
	t.Setenv("T_NUM", "not-an-int")
	t.Setenv("T_PTR", "also-bad")

	var buf bytes.Buffer
	w := withText{Num: 5}

	require.NoError(t, config.LoadFromEnv(&w, "T_", config.WithLogger(newCapturingLogger(&buf))))

	assert.Equal(t, numText(5), w.Num, "failed unmarshal keeps the prior value")
	require.NotNil(t, w.Ptr, "pointer is allocated before the failed unmarshal")
	assert.Equal(t, numText(0), *w.Ptr, "failed unmarshal leaves the zero value")

	out := buf.String()
	assert.Contains(t, out, "T_NUM")
	assert.Contains(t, out, "T_PTR")
}

func TestLoadFromEnv_TextUnmarshalerErrorStrict(t *testing.T) {
	t.Setenv("T_NUM", "not-an-int")
	t.Setenv("T_PTR", "also-bad")

	var w withText
	err := config.LoadFromEnv(&w, "T_", config.WithStrict())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "T_NUM")
	assert.Contains(t, err.Error(), "T_PTR")
}

func TestLoadFromEnv_UnsupportedSliceKindIgnored(t *testing.T) {
	// Only []string is bindable; any other slice element kind is left untouched
	// rather than misparsed.
	type withIntSlice struct {
		Nums []int `env:"NUMS"`
	}

	t.Setenv("U_NUMS", "1,2,3")

	w := withIntSlice{Nums: []int{9}}
	require.NoError(t, config.LoadFromEnv(&w, "U_"))
	assert.Equal(t, []int{9}, w.Nums, "non-string slices are not bindable and keep their value")
}

func TestLoadFromEnv_UnsupportedScalarKindIgnored(t *testing.T) {
	// Maps fall through setScalar's default case and are silently skipped.
	type withMap struct {
		M map[string]string `env:"M"`
	}

	t.Setenv("M_M", "a=b")

	w := withMap{M: map[string]string{"keep": "me"}}
	require.NoError(t, config.LoadFromEnv(&w, "M_"))
	assert.Equal(t, map[string]string{"keep": "me"}, w.M)
}

func TestLoadFromEnv_CombinedOptions(t *testing.T) {
	// WithTag, WithStrict, and WithLogger must compose: a custom tag is honored
	// and a malformed value under it is reported in strict mode.
	type custom struct {
		Name string `cfg:"NAME"`
		Num  int    `cfg:"NUM"`
	}

	t.Setenv("CO_NAME", "tagged")
	t.Setenv("CO_NUM", "not-a-number")

	var buf bytes.Buffer
	var c custom
	err := config.LoadFromEnv(
		&c, "CO_",
		config.WithTag("cfg"),
		config.WithStrict(),
		config.WithLogger(newCapturingLogger(&buf)),
	)

	require.Error(t, err)
	assert.Equal(t, "tagged", c.Name, "the valid field still binds")
	assert.Contains(t, err.Error(), "CO_NUM")
	assert.Empty(t, buf.String(), "strict mode returns errors rather than logging them")
}

func TestLoadFromEnv_DeeplyNestedPointerStructs(t *testing.T) {
	type leaf struct {
		Val string `env:"VAL"`
	}

	type mid struct {
		Leaf *leaf
	}

	type top struct {
		Mid mid
	}

	t.Setenv("N_VAL", "deep")

	tp := top{Mid: mid{Leaf: &leaf{}}}
	require.NoError(t, config.LoadFromEnv(&tp, "N_"))

	require.NotNil(t, tp.Mid.Leaf)
	assert.Equal(t, "deep", tp.Mid.Leaf.Val)
}

func TestLoadFromEnv_AllStrictErrorsJoined(t *testing.T) {
	// Every malformed kind must surface in the joined strict error, not just the
	// first one encountered.
	t.Setenv("A_FLAG", "notbool")
	t.Setenv("A_I", "x")
	t.Setenv("A_U", "-1") // negative is invalid for unsigned
	t.Setenv("A_F64", "nan-ish")
	t.Setenv("A_DUR", "10")  // missing unit
	t.Setenv("A_I8", "9000") // overflow

	var s scalars
	err := config.LoadFromEnv(&s, "A_", config.WithStrict())

	require.Error(t, err)
	for _, name := range []string{"A_FLAG", "A_I", "A_U", "A_F64", "A_DUR", "A_I8"} {
		assert.Contains(t, err.Error(), name, "expected %s in joined error", name)
	}
}

func TestLoadFromEnv_NegativeIntoSignedSucceeds(t *testing.T) {
	// A negative value is valid for a signed field and must bind cleanly.
	t.Setenv("NEG_I", "-42")
	t.Setenv("NEG_DUR", "-5s")

	var s scalars
	require.NoError(t, config.LoadFromEnv(&s, "NEG_"))
	assert.Equal(t, -42, s.I)
	assert.Equal(t, -5*time.Second, s.Dur)
}
