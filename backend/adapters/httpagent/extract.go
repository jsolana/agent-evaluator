package httpagent

import (
	"strconv"
	"strings"
)

// lookup follows a dotted path ("data.reply", "choices.0.message.content")
// through decoded JSON. Numbers index arrays, anything else names a field.
func lookup(v any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	for _, key := range strings.Split(path, ".") {
		switch node := v.(type) {
		case map[string]any:
			next, ok := node[key]
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			v = node[i]
		default:
			return nil, false
		}
	}
	return v, true
}

func lookupString(v any, path string) (string, bool) {
	got, ok := lookup(v, path)
	s, isString := got.(string)
	return s, ok && isString
}

// lookupInt reads a JSON number (decoded as float64) as an int; 0 when absent.
func lookupInt(v any, path string) int {
	got, _ := lookup(v, path)
	if f, ok := got.(float64); ok {
		return int(f)
	}
	return 0
}
