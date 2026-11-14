package criterionkit

import (
	"context"
	"encoding/json"
	"testing"
)

type nop struct{}

func (nop) Evaluate(context.Context, Input) (Result, error) { return Result{Passed: true}, nil }

func okFactory(json.RawMessage) (Criterion, error) { return nop{}, nil }

func TestRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("b", okFactory)
	r.Register("a", okFactory)
	if got := r.Names(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("names = %v", got)
	}
	if _, ok := r.Lookup("a"); !ok {
		t.Fatal("a must be found")
	}
	if _, ok := r.Lookup("zzz"); ok {
		t.Fatal("an unknown type must not be found")
	}
}

func TestRegisterPanicsOnMisuse(t *testing.T) {
	for name, fn := range map[string]func(*Registry){
		"duplicate": func(r *Registry) { r.Register("a", okFactory); r.Register("a", okFactory) },
		"empty":     func(r *Registry) { r.Register("", okFactory) },
		"nil":       func(r *Registry) { r.Register("a", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic")
				}
			}()
			fn(NewRegistry())
		})
	}
}

func TestDecodeParamsRejectsUnknownFieldsAndAcceptsEmpty(t *testing.T) {
	var p struct{ Limit int }
	if err := DecodeParams(nil, &p); err != nil {
		t.Fatalf("empty params are valid: %v", err)
	}
	if err := DecodeParams(json.RawMessage(`{"Limit":3}`), &p); err != nil || p.Limit != 3 {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if err := DecodeParams(json.RawMessage(`{"limt":3}`), &p); err == nil {
		t.Fatal("a typo must be rejected")
	}
}
