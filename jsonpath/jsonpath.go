// Package jsonpath addresses values inside a decoded-JSON tree
// (map[string]any / []any, as produced by encoding/json into an any) using a
// small, dependency-free subset of JSONPath. It supports dotted member access
// ($.a.b), bracketed member access ($['a']["b"]), and numeric indexing ($.a[0]).
// A non-resolving path yields no value rather than an error, so it is safe to
// probe optional fields.
package jsonpath

import (
	"strconv"
	"strings"
)

// segment is one step of a parsed path: a map key or an array index.
type segment struct {
	key     string
	index   int
	isIndex bool
}

// Eval addresses a value in a decoded-JSON tree. It supports dotted member
// access ($.a.b), bracketed access ($['a']['b']), and numeric indexing
// ($.a[0]). It returns (value, true) on a resolving path, or (nil, false) when
// the path does not resolve — a non-resolving path simply yields no value.
//
//nolint:gocognit // one walk over the parsed path segments
func Eval(root any, path string) (any, bool) {
	segments, ok := parsePath(path)
	if !ok {
		return nil, false
	}
	cur := root
	for _, seg := range segments {
		if seg.isIndex {
			arr, isArr := cur.([]any)
			if !isArr || seg.index < 0 || seg.index >= len(arr) {
				return nil, false
			}
			cur = arr[seg.index]
			continue
		}
		obj, isObj := cur.(map[string]any)
		if !isObj {
			return nil, false
		}
		next, exists := obj[seg.key]
		if !exists {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// parsePath tokenizes a path into segments. It returns false for any syntax
// outside the supported subset.
func parsePath(path string) ([]segment, bool) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$")

	var segments []segment
	for len(path) > 0 {
		switch path[0] {
		case '.':
			path = path[1:]
			key := readIdent(&path)
			if key == "" {
				return nil, false
			}
			segments = append(segments, segment{key: key})
		case '[':
			seg, rest, ok := readBracket(path)
			if !ok {
				return nil, false
			}
			segments = append(segments, seg)
			path = rest
		default:
			return nil, false
		}
	}
	return segments, true
}

// readIdent consumes a leading identifier from *path, advancing it.
func readIdent(path *string) string {
	s := *path
	i := 0
	for i < len(s) && s[i] != '.' && s[i] != '[' {
		i++
	}
	ident := s[:i]
	*path = s[i:]
	return ident
}

// readBracket parses one [...] accessor — a quoted key or a numeric index.
func readBracket(path string) (segment, string, bool) {
	end := strings.IndexByte(path, ']')
	if end < 0 {
		return segment{}, "", false
	}
	inner := path[1:end]
	rest := path[end+1:]

	if len(inner) >= 2 && (inner[0] == '\'' || inner[0] == '"') && inner[len(inner)-1] == inner[0] {
		return segment{key: inner[1 : len(inner)-1]}, rest, true
	}
	idx, err := strconv.Atoi(strings.TrimSpace(inner))
	if err != nil {
		return segment{}, "", false
	}
	return segment{index: idx, isIndex: true}, rest, true
}
