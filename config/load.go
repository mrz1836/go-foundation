package config

import (
	"encoding"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// errLoadTarget is returned when LoadFromEnv is given something other than a
// non-nil pointer to a struct.
var errLoadTarget = errors.New("config: LoadFromEnv requires a non-nil pointer to a struct")

// durationType is the reflect.Type of time.Duration, used to special-case
// duration parsing ahead of the generic integer handling (a Duration is an
// int64 under the hood).
//
//nolint:gochecknoglobals // reflect.Type lookup cached once to avoid recomputation per field
var durationType = reflect.TypeOf(time.Duration(0))

// textUnmarshalerType is the reflect.Type of encoding.TextUnmarshaler. Any field
// whose pointer implements it self-parses, which is the open/closed extension
// point: new value types become bindable without changing this package.
//
//nolint:gochecknoglobals // reflect.Type lookup cached once to avoid recomputation per field
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// loadOptions holds the resolved configuration for a LoadFromEnv call.
type loadOptions struct {
	logger *slog.Logger
	tag    string
	strict bool
}

// Option customizes LoadFromEnv behavior.
type Option func(*loadOptions)

// WithLogger sets the logger used to report ignored, malformed values in the
// default (lenient) mode. When nil, slog.Default() is used.
func WithLogger(l *slog.Logger) Option {
	return func(o *loadOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithTag overrides the struct tag key that carries the environment variable
// name. The default is "env".
func WithTag(tag string) Option {
	return func(o *loadOptions) {
		if tag != "" {
			o.tag = tag
		}
	}
}

// WithStrict makes LoadFromEnv return an error for every malformed value rather
// than logging it and keeping the field's existing value. Use it on hosts that
// should fail fast at startup on a bad environment variable.
func WithStrict() Option {
	return func(o *loadOptions) {
		o.strict = true
	}
}

// LoadFromEnv applies environment-variable overrides to cfg, which must be a
// non-nil pointer to a struct.
//
// For every field carrying a tag of the form `env:"NAME"`, the loader reads
// os.Getenv(prefix+NAME); when that value is non-empty it is parsed and assigned
// to the field. The prefix lets each consuming service own its environment
// namespace (for example "MYAPP_") while the shared config types keep generic,
// prefix-less tags. Nested structs — including anonymous embedded ones — are
// walked recursively, so a service that embeds config.Config binds the shared
// fields and its own fields in a single call.
//
// Supported field kinds: string, bool, all sized int/uint and float kinds,
// time.Duration ("30s"), []string (comma-separated), and any type whose pointer
// implements encoding.TextUnmarshaler. Unexported fields and unsupported kinds
// are skipped.
//
// By default the loader is lenient: a value that fails to parse is logged and
// the field keeps its prior value (typically the JSON default). WithStrict makes
// such failures return a joined error instead.
func LoadFromEnv(cfg any, prefix string, opts ...Option) error {
	o := loadOptions{logger: slog.Default(), tag: "env"}
	for _, apply := range opts {
		apply(&o)
	}

	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("%w: got %T", errLoadTarget, cfg)
	}

	root := rv.Elem()
	if root.Kind() != reflect.Struct {
		return fmt.Errorf("%w: got pointer to %s", errLoadTarget, root.Kind())
	}

	var errs []error
	walkStruct(root, prefix, &o, &errs)

	return errors.Join(errs...)
}

// walkStruct visits every field of a struct value, binding env-tagged leaves and
// recursing into nested or embedded structs that carry no tag of their own.
func walkStruct(v reflect.Value, prefix string, o *loadOptions, errs *[]error) {
	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		tag := t.Field(i).Tag.Get(o.tag)

		if tag == "" {
			recurseContainer(field, prefix, o, errs)
			continue
		}

		if !field.CanSet() {
			continue // unexported field; nothing we can assign to
		}

		raw := os.Getenv(prefix + tag)
		if raw == "" {
			continue // unset or empty: keep the existing (JSON-loaded) value
		}

		bindField(field, prefix+tag, raw, o, errs)
	}
}

// recurseContainer walks into an untagged struct, or a non-nil pointer to a
// struct, so nested configuration is bound. Other untagged fields are ignored.
func recurseContainer(field reflect.Value, prefix string, o *loadOptions, errs *[]error) {
	//nolint:exhaustive // only struct and pointer-to-struct containers are walked
	switch field.Kind() {
	case reflect.Struct:
		walkStruct(field, prefix, o, errs)
	case reflect.Pointer:
		if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
			walkStruct(field.Elem(), prefix, o, errs)
		}
	default:
		// Untagged scalars, slices, maps, etc. are not bindable on their own.
	}
}

// bindField assigns a single raw environment value to an env-tagged field,
// honoring pointers, encoding.TextUnmarshaler, and time.Duration before falling
// back to kind-based scalar parsing.
func bindField(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	// Allocate and dereference a pointer target so the value lands in *T.
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}

		field = field.Elem()
	}

	if tryTextUnmarshaler(field, name, raw, o, errs) {
		return
	}

	if field.Type() == durationType {
		setDuration(field, name, raw, o, errs)
		return
	}

	setScalar(field, name, raw, o, errs)
}

// tryTextUnmarshaler binds the value via encoding.TextUnmarshaler when the field's
// pointer implements it, reporting any parse failure. It returns true when it
// handled the field so the caller can stop.
func tryTextUnmarshaler(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) bool {
	if !field.CanAddr() || !field.Addr().Type().Implements(textUnmarshalerType) {
		return false
	}

	u, _ := field.Addr().Interface().(encoding.TextUnmarshaler)
	if err := u.UnmarshalText([]byte(raw)); err != nil {
		report(o, errs, name, raw, err)
	}

	return true
}

// setDuration parses a time.Duration override (for example "30s" or "5m").
func setDuration(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		report(o, errs, name, raw, err)
		return
	}

	field.SetInt(int64(d))
}

// setScalar assigns a raw value based on the field's reflect.Kind. Parse and
// overflow failures are routed through report so they honor strict/lenient mode.
func setScalar(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	//nolint:exhaustive // unsupported kinds are intentionally ignored via the default case
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		setBool(field, name, raw, o, errs)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		setInt(field, name, raw, o, errs)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		setUint(field, name, raw, o, errs)
	case reflect.Float32, reflect.Float64:
		setFloat(field, name, raw, o, errs)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(splitCommaList(raw)))
		}
	default:
		// Unsupported field kind: leave the existing value untouched.
	}
}

// setBool parses a boolean override (1, t, T, TRUE, true, etc.).
func setBool(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	b, err := strconv.ParseBool(raw)
	if err != nil {
		report(o, errs, name, raw, err)
		return
	}

	field.SetBool(b)
}

// setInt parses a signed integer override and guards against overflow for the
// field's specific width.
func setInt(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	n, err := strconv.ParseInt(raw, 10, field.Type().Bits())
	if err != nil {
		report(o, errs, name, raw, err)
		return
	}

	field.SetInt(n)
}

// setUint parses an unsigned integer override and guards against overflow for the
// field's specific width.
func setUint(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	n, err := strconv.ParseUint(raw, 10, field.Type().Bits())
	if err != nil {
		report(o, errs, name, raw, err)
		return
	}

	field.SetUint(n)
}

// setFloat parses a floating-point override.
func setFloat(field reflect.Value, name, raw string, o *loadOptions, errs *[]error) {
	f, err := strconv.ParseFloat(raw, field.Type().Bits())
	if err != nil {
		report(o, errs, name, raw, err)
		return
	}

	field.SetFloat(f)
}

// report records a malformed value: it returns an error in strict mode or logs a
// warning and continues in the default lenient mode.
func report(o *loadOptions, errs *[]error, name, raw string, err error) {
	if o.strict {
		*errs = append(*errs, fmt.Errorf("config: env %s=%q: %w", name, raw, err))
		return
	}

	o.logger.Warn("config: ignoring invalid environment override",
		"var", name, "value", raw, "err", err)
}

// splitCommaList splits a comma-separated value into a trimmed, empty-free slice.
func splitCommaList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// ApplyReadDatabaseFallback copies the write-database settings into read when no
// read host is configured. This is convenient for local and test environments
// that run a single PostgreSQL instance with no dedicated read replica.
//
// It operates on the building-block database configs (rather than a whole Config)
// so it composes with any configuration layout — whether a service embeds the
// shared Config or declares its own database fields of these types.
func ApplyReadDatabaseFallback(write WriteDatabaseConfig, read *ReadDatabaseConfig) {
	if read == nil || read.Host != "" {
		return
	}

	*read = ReadDatabaseConfig(write)
}
