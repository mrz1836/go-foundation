package secrets_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/mrz1836/go-foundation/secrets"
)

// ExampleValue shows that a Value prints, logs, and marshals as [REDACTED],
// and that only Reveal returns the secret.
func ExampleValue() {
	key := secrets.NewValue("example-api-key")

	fmt.Println(key)
	fmt.Printf("%q %x %+v\n", key, key, struct{ Key secrets.Value }{key})

	out, err := json.Marshal(map[string]secrets.Value{"key": key})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(out))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return a
		},
	}))
	logger.Info("calling the API", "key", key)

	fmt.Println(len(key.Reveal()))
	// Output:
	// [REDACTED]
	// [REDACTED] [REDACTED] {Key:[REDACTED]}
	// {"key":"[REDACTED]"}
	// level=INFO msg="calling the API" key=[REDACTED]
	// 15
}
