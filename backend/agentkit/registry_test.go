package agentkit

import (
	"context"
	"strings"
	"testing"
)

type stub struct{}

func (stub) NewConversation(context.Context, ConversationSpec) (Conversation, error) { return nil, nil }

func okFactory(Env) (Plugin, error) { return Plugin{Agent: stub{}}, nil }

func TestBuildUnknownNameListsRegisteredOnes(t *testing.T) {
	r := NewRegistry()
	r.Register("b", okFactory)
	r.Register("a", okFactory)
	_, err := r.Build("zzz", nil)
	if err == nil || !strings.Contains(err.Error(), "a, b") {
		t.Fatalf("want the known names listed, got %v", err)
	}
	if _, err := r.Build("", nil); err == nil {
		t.Fatal("an empty name must fail instead of picking a default")
	}
}

func TestBuildFillsMissingTraces(t *testing.T) {
	r := NewRegistry()
	r.Register("a", okFactory)
	p, err := r.Build("a", nil)
	if err != nil || p.Traces == nil {
		t.Fatalf("plugin=%+v err=%v", p, err)
	}
}

func TestBuildRejectsFactoriesWithoutAgent(t *testing.T) {
	r := NewRegistry()
	r.Register("a", func(Env) (Plugin, error) { return Plugin{}, nil })
	if _, err := r.Build("a", nil); err == nil {
		t.Fatal("a factory returning no agent must fail")
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

func TestRequireReportsTheMissingSetting(t *testing.T) {
	env := func(k string) string { return map[string]string{"A": "x"}[k] }
	if err := Require(env, "A")(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Require(env, "A", "B")(context.Background()); err == nil || !strings.Contains(err.Error(), "B is not set") {
		t.Fatalf("got %v", err)
	}
}
