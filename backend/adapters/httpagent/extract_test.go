package httpagent

import (
	"encoding/json"
	"testing"
)

func TestLookupFollowsFieldsAndArrayIndexes(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`{"a":{"b":[{"c":"deep"},{"c":"second"}]},"n":7,"s":"x"}`), &v)
	for path, want := range map[string]string{"a.b.0.c": "deep", "a.b.1.c": "second", "s": "x"} {
		if got, ok := lookupString(v, path); !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", path, got, ok, want)
		}
	}
	for _, path := range []string{"", "a.b.2.c", "a.b.x", "a.z", "s.deeper", "n"} {
		if got, ok := lookupString(v, path); ok {
			t.Errorf("%q must not resolve to a string, got %q", path, got)
		}
	}
	if lookupInt(v, "n") != 7 || lookupInt(v, "missing") != 0 {
		t.Fatal("numbers are read as ints and absent ones as 0")
	}
}
