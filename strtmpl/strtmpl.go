// Package strtmpl renders {{name}} placeholders in a string from one or more
// value maps. It is deliberately smaller and safer than text/template for the
// common "fill in named variables" case:
//
//   - Placeholders are identified once from the original template, so a value
//     substituted for one placeholder is never itself re-scanned for further
//     placeholders. A caller-supplied value shaped like "{{secret}}" can
//     therefore never resolve against a later map — the classic injection an
//     iterative replacer allows.
//   - Multiple sources are consulted in order, first match wins, so a caller can
//     layer trust tiers (for example public values first, secret values second)
//     and a low-trust map can never shadow a value only the high-trust map holds.
//
// It is dependency-free.
package strtmpl

import (
	"regexp"
	"strings"
)

// placeholderPattern matches a {{name}} placeholder with optional inner spaces.
var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// Render substitutes {{name}} placeholders in tmpl using sources. Placeholders
// are identified once from the original template (never re-scanned from a
// substituted value); sources are consulted in order and the first match wins.
// A placeholder found in no source renders as the empty string and is reported
// in unknown, which lists one entry per unresolved occurrence in the order they
// appear (a strict caller can treat a non-empty unknown as an error).
func Render(tmpl string, sources ...map[string]string) (string, []string) {
	var unknown []string

	out := placeholderPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := strings.TrimFunc(match[2:len(match)-2], func(r rune) bool {
			return r == ' ' || r == '\t'
		})
		for _, src := range sources {
			if v, ok := src[name]; ok {
				return v
			}
		}
		unknown = append(unknown, name)
		return ""
	})

	return out, unknown
}
