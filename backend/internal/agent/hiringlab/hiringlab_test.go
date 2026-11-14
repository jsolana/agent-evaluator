package hiringlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/internal/agent"
)

type askCall struct {
	auth, key string
	body      map[string]any
}

// fakeAsk imitates POST /ask: it records calls and replies with status[i] (200 when absent).
type fakeAsk struct {
	mu            sync.Mutex
	calls         []askCall
	status        []int
	conversations []map[string]any // bodies of POST /conversations
	convStatus    int              // status for POST /conversations (default 201)
}

func (f *fakeAsk) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if r.URL.Path == "/proj/conversations" && r.Method == http.MethodPost {
		f.conversations = append(f.conversations, body)
		if f.convStatus != 0 {
			w.WriteHeader(f.convStatus)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "conv-tagged", "kind": "conversation"})
		return
	}
	f.calls = append(f.calls, askCall{r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), body})
	if r.URL.Path != "/proj/ask" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if n := len(f.calls); n <= len(f.status) && f.status[n-1] != 200 {
		w.WriteHeader(f.status[n-1])
		_, _ = w.Write([]byte(`{"error":"busy","conversationId":"conv-1"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"answer":         `{"text_response":"Hola **cliente**"}`,
		"conversationId": "conv-1",
		"tokens":         120,
		"durationMs":     900,
		"traces":         []any{},
		"snapshot":       map[string]any{"promptId": "p1", "model": "gpt-6-luna"},
	})
}

func newAgent(t *testing.T, f *fakeAsk) (*Agent, func()) {
	srv := httptest.NewServer(f)
	return New(Config{BaseURL: srv.URL + "/proj", Token: "secret"}), srv.Close
}

func TestConversationKeepsContextAcrossTurns(t *testing.T) {
	f := &fakeAsk{}
	a, stop := newAgent(t, f)
	defer stop()

	conv, err := a.NewConversation(context.Background(), agent.ConversationSpec{Config: json.RawMessage(`{"promptId":"p1"}`)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := conv.Send(context.Background(), agent.TurnInput{Index: 1, Message: "hola", IdempotencyKey: "key-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "Hola **cliente**" || out.Tokens != 120 || out.DurationMs != 900 || conv.Ref() != "conv-1" {
		t.Fatalf("unexpected output %+v ref=%s", out, conv.Ref())
	}
	if string(out.EffectiveConfig) != `{"model":"gpt-6-luna","promptId":"p1"}` || len(out.Raw) == 0 {
		t.Fatalf("snapshot and raw must be kept: %s", out.EffectiveConfig)
	}
	if _, err := conv.Send(context.Background(), agent.TurnInput{Index: 2, Message: "y?", IdempotencyKey: "key-0002"}); err != nil {
		t.Fatal(err)
	}

	first, second := f.calls[0], f.calls[1]
	if first.auth != "Bearer secret" || first.key != "key-0001" || first.body["promptId"] != "p1" {
		t.Fatalf("bad first request: %+v", first)
	}
	if _, ok := first.body["conversationId"]; ok {
		t.Fatal("first turn must not send a conversationId")
	}
	if second.body["conversationId"] != "conv-1" || second.key != "key-0002" {
		t.Fatalf("second turn must continue the conversation: %+v", second)
	}
}

func TestResumeUsesExistingConversation(t *testing.T) {
	f := &fakeAsk{}
	a, stop := newAgent(t, f)
	defer stop()
	conv, _ := a.NewConversation(context.Background(), agent.ConversationSpec{Ref: "conv-9"})
	_, _ = conv.Send(context.Background(), agent.TurnInput{Message: "x", IdempotencyKey: "key-0003"})
	if f.calls[0].body["conversationId"] != "conv-9" {
		t.Fatalf("resumed conversation must send its ref: %v", f.calls[0].body)
	}
}

func TestErrorClassification(t *testing.T) {
	tests := map[int]error{409: agent.ErrConflict, 502: agent.ErrRetryable, 504: agent.ErrRetryable, 400: agent.ErrFatal, 401: agent.ErrFatal}
	for status, want := range tests {
		f := &fakeAsk{status: []int{status}}
		a, stop := newAgent(t, f)
		conv, _ := a.NewConversation(context.Background(), agent.ConversationSpec{})
		_, err := conv.Send(context.Background(), agent.TurnInput{Message: "x", IdempotencyKey: "key-0004"})
		stop()
		if !errors.Is(err, want) {
			t.Errorf("status %d: got %v, want %v", status, err, want)
		}
		if status == 409 && conv.Ref() != "conv-1" {
			t.Errorf("a reserved turn's conversationId must be captured even on error")
		}
	}
}

func TestNetworkErrorIsRetryable(t *testing.T) {
	a := New(Config{BaseURL: "http://127.0.0.1:1/p", Token: "t"})
	conv, _ := a.NewConversation(context.Background(), agent.ConversationSpec{})
	_, err := conv.Send(context.Background(), agent.TurnInput{Message: "x", IdempotencyKey: "key-0005"})
	if !errors.Is(err, agent.ErrRetryable) {
		t.Fatalf("got %v", err)
	}
}

func TestConfigIsValidated(t *testing.T) {
	a := New(Config{BaseURL: "http://x/p", Token: "t"})
	if _, err := a.NewConversation(context.Background(), agent.ConversationSpec{Config: json.RawMessage(`{"prompt_id":"typo"}`)}); err == nil {
		t.Fatal("unknown config fields must be rejected")
	}
}

func TestPlainTextAnswerPassesThrough(t *testing.T) {
	if got := answerText(`plain answer`); got != "plain answer" {
		t.Fatal(got)
	}
}

func TestLabelsCreateATaggedConversation(t *testing.T) {
	f := &fakeAsk{}
	a, stop := newAgent(t, f)
	defer stop()
	conv, err := a.NewConversation(context.Background(), agent.ConversationSpec{Labels: map[string]string{"evaluation": "7", "scenario": "mortgage"}})
	if err != nil {
		t.Fatal(err)
	}
	if conv.Ref() != "conv-tagged" || len(f.conversations) != 1 {
		t.Fatalf("ref=%q conversations=%v", conv.Ref(), f.conversations)
	}
	data, _ := f.conversations[0]["data"].(map[string]any)
	meta, _ := data["metadata"].(map[string]any)
	if meta["scenario"] != "mortgage" || meta["evaluation"] != "7" || data["title"] == "" {
		t.Fatalf("tagging body = %v", f.conversations[0])
	}
	_, _ = conv.Send(context.Background(), agent.TurnInput{Message: "x", IdempotencyKey: "key-0006"})
	if f.calls[0].body["conversationId"] != "conv-tagged" {
		t.Fatalf("first turn must use the tagged conversation: %v", f.calls[0].body)
	}
}

func TestTaggingIsBestEffortAndSkippedOnResume(t *testing.T) {
	f := &fakeAsk{convStatus: 500}
	a, stop := newAgent(t, f)
	defer stop()
	conv, err := a.NewConversation(context.Background(), agent.ConversationSpec{Labels: map[string]string{"scenario": "s"}})
	if err != nil || conv.Ref() != "" {
		t.Fatalf("a tagging failure must fall back to an untagged conversation: ref=%q err=%v", conv.Ref(), err)
	}
	resumed, _ := a.NewConversation(context.Background(), agent.ConversationSpec{Ref: "conv-1", Labels: map[string]string{"scenario": "s"}})
	if resumed.Ref() != "conv-1" || len(f.conversations) != 1 {
		t.Fatalf("resume must not create a new conversation: %v", f.conversations)
	}
}
