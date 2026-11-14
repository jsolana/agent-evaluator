package domain

import (
	"encoding/json"
	"testing"
)

func TestSpanArguments(t *testing.T) {
	for name, tc := range map[string]struct {
		attributes string
		want       string
	}{
		"arguments set":      {`{"arguments":{"query":"x"},"result":[1]}`, `{"query":"x"}`},
		"no arguments":       {`{"result":[1]}`, ``},
		"no attributes":      {``, ``},
		"invalid attributes": {`not json`, ``},
	} {
		t.Run(name, func(t *testing.T) {
			s := Span{Attributes: json.RawMessage(tc.attributes)}
			if got := string(s.Arguments()); got != tc.want {
				t.Fatalf("Arguments() = %q, want %q", got, tc.want)
			}
		})
	}
}
