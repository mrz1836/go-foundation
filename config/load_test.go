package config_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/config"
)

// scalars exercises every field kind the loader supports.
type scalars struct {
	Str      string        `env:"STR"`
	Flag     bool          `env:"FLAG"`
	I        int           `env:"I"`
	I8       int8          `env:"I8"`
	I64      int64         `env:"I64"`
	U        uint          `env:"U"`
	F64      float64       `env:"F64"`
	Dur      time.Duration `env:"DUR"`
	List     []string      `env:"LIST"`
	Untagged string        // no env tag: never bound
	unexp    string        `env:"UNEXP"`
}

func TestLoadFromEnv_AppliesEverySupportedKind(t *testing.T) {
	t.Setenv("APP_STR", "hello")
	t.Setenv("APP_FLAG", "true")
	t.Setenv("APP_I", "42")
	t.Setenv("APP_I8", "7")
	t.Setenv("APP_I64", "9000000000")
	t.Setenv("APP_U", "12")
	t.Setenv("APP_F64", "3.14")
	t.Setenv("APP_DUR", "90s")
	t.Setenv("APP_LIST", " a , b ,, c ")
	t.Setenv("APP_UNEXP", "ignored")

	var s scalars
	require.NoError(t, config.LoadFromEnv(&s, "APP_"))

	assert.Equal(t, "hello", s.Str)
	assert.True(t, s.Flag)
	assert.Equal(t, 42, s.I)
	assert.Equal(t, int8(7), s.I8)
	assert.Equal(t, int64(9000000000), s.I64)
	assert.Equal(t, uint(12), s.U)
	assert.InDelta(t, 3.14, s.F64, 1e-9)
	assert.Equal(t, 90*time.Second, s.Dur)
	assert.Equal(t, []string{"a", "b", "c"}, s.List)
	assert.Empty(t, s.unexp, "unexported fields must be skipped")
}

func TestLoadFromEnv_EmptyAndUnsetKeepExistingValue(t *testing.T) {
	s := scalars{Str: "default", I: 5}

	t.Setenv("APP_STR", "") // explicitly empty: treated as unset
	// APP_I is never set.

	require.NoError(t, config.LoadFromEnv(&s, "APP_"))
	assert.Equal(t, "default", s.Str)
	assert.Equal(t, 5, s.I)
}

func TestLoadFromEnv_PrefixIsolatesNamespaces(t *testing.T) {
	t.Setenv("DB_WRITE_HOST", "wrong") // no prefix: must be ignored
	t.Setenv("APP_DB_WRITE_HOST", "right")

	type wrap struct {
		Host string `env:"DB_WRITE_HOST"`
	}

	var w wrap
	require.NoError(t, config.LoadFromEnv(&w, "APP_"))
	assert.Equal(t, "right", w.Host)
}

// projectConfig mirrors the real consumer pattern: embed the shared config and
// add project-specific fields with prefix-less tags.
type projectConfig struct {
	config.Config

	Scout scoutConfig `json:"scout"`
}

type scoutConfig struct {
	APIKey string `json:"api_key" env:"ANTHROPIC_API_KEY"`
}

func TestLoadFromEnv_WalksEmbeddedAndNestedStructs(t *testing.T) {
	t.Setenv("BEDROCK_DB_WRITE_HOST", "db.internal")
	t.Setenv("BEDROCK_DB_WRITE_PORT", "5432")
	t.Setenv("BEDROCK_APP_NAME", "bedrock-api")
	t.Setenv("BEDROCK_ANTHROPIC_API_KEY", "sk-test")

	var cfg projectConfig
	require.NoError(t, config.LoadFromEnv(&cfg, "BEDROCK_"))

	assert.Equal(t, "db.internal", cfg.WriteDatabase.Host)
	assert.Equal(t, 5432, cfg.WriteDatabase.Port)
	assert.Equal(t, "bedrock-api", cfg.Application.Name)
	assert.Equal(t, "sk-test", cfg.Scout.APIKey)
}

func TestLoadFromEnv_PointerStructs(t *testing.T) {
	type inner struct {
		Val string `env:"VAL"`
	}

	type outer struct {
		Set   *inner
		Unset *inner
	}

	t.Setenv("P_VAL", "bound")

	o := outer{Set: &inner{}} // non-nil: walked; Unset stays nil and is skipped
	require.NoError(t, config.LoadFromEnv(&o, "P_"))

	require.NotNil(t, o.Set)
	assert.Equal(t, "bound", o.Set.Val)
	assert.Nil(t, o.Unset)
}

// upperString self-parses via encoding.TextUnmarshaler, proving the open/closed
// extension point: a new value type becomes bindable without touching the loader.
type upperString string

func (u *upperString) UnmarshalText(text []byte) error {
	*u = upperString(strings.ToUpper(string(text)))
	return nil
}

// numText fails to unmarshal on non-numeric input, exercising the error path.
type numText int

func (n *numText) UnmarshalText(text []byte) error {
	v, err := strconv.Atoi(string(text))
	if err != nil {
		return err
	}

	*n = numText(v)

	return nil
}

func TestLoadFromEnv_TextUnmarshaler(t *testing.T) {
	type withText struct {
		Up  upperString  `env:"UP"`
		Ptr *upperString `env:"PTR"`
		Num numText      `env:"NUM"`
	}

	t.Setenv("X_UP", "hello")
	t.Setenv("X_PTR", "world")
	t.Setenv("X_NUM", "10")

	var w withText
	require.NoError(t, config.LoadFromEnv(&w, "X_"))

	assert.Equal(t, upperString("HELLO"), w.Up)
	require.NotNil(t, w.Ptr)
	assert.Equal(t, upperString("WORLD"), *w.Ptr)
	assert.Equal(t, numText(10), w.Num)
}

func TestLoadFromEnv_StrictReportsErrorsLenientKeepsValue(t *testing.T) {
	t.Setenv("S_I", "not-a-number")
	t.Setenv("S_DUR", "not-a-duration")

	// Lenient (default): malformed values are skipped, prior values survive.
	lenient := scalars{I: 1, Dur: time.Second}
	require.NoError(t, config.LoadFromEnv(&lenient, "S_"))
	assert.Equal(t, 1, lenient.I)
	assert.Equal(t, time.Second, lenient.Dur)

	// Strict: every malformed value is reported.
	strict := scalars{I: 1, Dur: time.Second}
	err := config.LoadFromEnv(&strict, "S_", config.WithStrict())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S_I")
	assert.Contains(t, err.Error(), "S_DUR")
}

func TestLoadFromEnv_OverflowGuarded(t *testing.T) {
	t.Setenv("O_I8", "999") // out of int8 range

	s := scalars{I8: 3}
	require.NoError(t, config.LoadFromEnv(&s, "O_"))
	assert.Equal(t, int8(3), s.I8, "overflowing value must be ignored in lenient mode")

	s2 := scalars{I8: 3}
	require.Error(t, config.LoadFromEnv(&s2, "O_", config.WithStrict()))
}

func TestLoadFromEnv_WithTag(t *testing.T) {
	type custom struct {
		Name string `cfg:"NAME"`
	}

	t.Setenv("C_NAME", "tagged")

	var c custom
	require.NoError(t, config.LoadFromEnv(&c, "C_", config.WithTag("cfg")))
	assert.Equal(t, "tagged", c.Name)
}

func TestLoadFromEnv_InvalidTargets(t *testing.T) {
	tests := map[string]any{
		"nil":                   nil,
		"non-pointer":           scalars{},
		"pointer to non-struct": new(int),
	}

	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			err := config.LoadFromEnv(target, "APP_")
			require.Error(t, err)
		})
	}
}

func TestApplyReadDatabaseFallback(t *testing.T) {
	t.Run("copies write config when read host is empty", func(t *testing.T) {
		write := config.WriteDatabaseConfig{
			Host: "primary", Port: 5432, Database: "app", Username: "u",
			SSLMode: "require", MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 5, Driver: "pgx",
		}

		var read config.ReadDatabaseConfig

		config.ApplyReadDatabaseFallback(write, &read)

		assert.Equal(t, "primary", read.Host)
		assert.Equal(t, "pgx", read.Driver)
		assert.Equal(t, write.ConnectionString(), read.ConnectionString())
	})

	t.Run("leaves an explicit read host untouched", func(t *testing.T) {
		write := config.WriteDatabaseConfig{Host: "primary"}
		read := config.ReadDatabaseConfig{Host: "replica"}

		config.ApplyReadDatabaseFallback(write, &read)
		assert.Equal(t, "replica", read.Host)
	})

	t.Run("nil read is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() {
			config.ApplyReadDatabaseFallback(config.WriteDatabaseConfig{}, nil)
		})
	})
}

func FuzzLoadFromEnv(f *testing.F) {
	f.Add("plain")
	f.Add("123")
	f.Add("-5")
	f.Add("true")
	f.Add("30s")
	f.Add("3.14")
	f.Add(",,a,,b,,")
	f.Add("\xff\xfe")

	f.Fuzz(func(t *testing.T, raw string) {
		// The OS forbids NUL bytes in environment values; t.Setenv rejects them
		// before the loader is ever reached, so they are out of scope here.
		if strings.ContainsRune(raw, 0) {
			t.Skip()
		}

		t.Setenv("F_STR", raw)
		t.Setenv("F_FLAG", raw)
		t.Setenv("F_I", raw)
		t.Setenv("F_I8", raw)
		t.Setenv("F_U", raw)
		t.Setenv("F_F64", raw)
		t.Setenv("F_DUR", raw)
		t.Setenv("F_LIST", raw)

		// Must never panic, in either mode, for any input.
		var lenient scalars
		_ = config.LoadFromEnv(&lenient, "F_")

		var strict scalars
		if err := config.LoadFromEnv(&strict, "F_", config.WithStrict()); err != nil {
			// A reported error must be inspectable, never nil-wrapped.
			require.NotEmpty(t, err.Error())
			_ = errors.Unwrap(err)
		}
	})
}

func BenchmarkLoadFromEnv(b *testing.B) {
	b.Setenv("BEDROCK_DB_WRITE_HOST", "db.internal")
	b.Setenv("BEDROCK_DB_WRITE_PORT", "5432")
	b.Setenv("BEDROCK_APP_NAME", "bedrock-api")
	b.Setenv("BEDROCK_ANTHROPIC_API_KEY", "sk-test")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		var cfg projectConfig
		_ = config.LoadFromEnv(&cfg, "BEDROCK_")
	}
}
