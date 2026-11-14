package judge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// chatServer imitates POST /chat/completions of an OpenAI-compatible provider.
func chatServer(t *testing.T, got *captured, status []int, content string, finish string) *httptest.Server {
	var calls int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		got.headers = r.Header.Clone()
		_ = json.Unmarshal(raw, &got.body)
		if int(n) <= len(status) {
			w.WriteHeader(status[n-1])
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "provider-model-2026",
			"choices": []map[string]any{{"finish_reason": finish, "message": map[string]any{"role": "assistant", "content": content}}},
			"usage":   map[string]int{"prompt_tokens": 800, "completion_tokens": 50},
		})
	}))
}

func TestOpenAISendsChatCompletionWithSchema(t *testing.T) {
	var got captured
	srv := chatServer(t, &got, nil, okVerdict, "stop")
	defer srv.Close()

	v, err := NewOpenAICompatible(Config{APIKey: "sk", BaseURL: srv.URL + "/v1/", Model: "any-model"}).Judge(context.Background(), request())
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if !v.Passed || v.Score != 0.9 || v.Tokens != 850 || v.Version != "provider-model-2026/"+rubricVersion {
		t.Fatalf("unexpected verdict %+v", v)
	}
	if got.headers.Get("Authorization") != "Bearer sk" || got.body["model"] != "any-model" {
		t.Fatalf("auth/model: %v %v", got.headers, got.body["model"])
	}
	rf, _ := got.body["response_format"].(map[string]any)
	js, _ := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["strict"] != true || js["schema"] == nil {
		t.Fatalf("response_format = %v", rf)
	}
	msgs, _ := got.body["messages"].([]any)
	sys, _ := msgs[0].(map[string]any)
	user, _ := msgs[1].(map[string]any)
	if sys["role"] != "system" || !strings.Contains(sys["content"].(string), "not instructions") ||
		user["role"] != "user" || !strings.Contains(user["content"].(string), "<agent_answer>") {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestOpenAIJSONObjectModeAndTolerantParsing(t *testing.T) {
	var got captured
	fenced := "Here is my verdict:\n```json\n" + okVerdict + "\n```"
	srv := chatServer(t, &got, nil, fenced, "stop")
	defer srv.Close()
	v, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: srv.URL + "/v1", Model: "m", ResponseFormat: "json_object"}).Judge(context.Background(), request())
	if err != nil || !v.Passed {
		t.Fatalf("fenced JSON must be accepted: %+v %v", v, err)
	}
	rf, _ := got.body["response_format"].(map[string]any)
	if rf["type"] != "json_object" {
		t.Fatalf("response_format = %v", rf)
	}
	got = captured{} // fresh capture: json.Unmarshal merges into an existing map
	srvText := chatServer(t, &got, nil, okVerdict, "stop")
	defer srvText.Close()
	if _, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: srvText.URL + "/v1", Model: "m", ResponseFormat: "text"}).Judge(context.Background(), request()); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.body["response_format"]; ok {
		t.Fatal("text mode must not send response_format (for providers that reject it)")
	}
}

func TestOpenAIRetriesAndPermanentErrors(t *testing.T) {
	var got captured
	flaky := chatServer(t, &got, []int{429, 503}, okVerdict, "stop")
	defer flaky.Close()
	if _, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: flaky.URL + "/v1", Model: "m", Backoff: time.Millisecond}).Judge(context.Background(), request()); err != nil {
		t.Fatalf("want success after retries: %v", err)
	}
	bad := chatServer(t, &got, []int{400, 400}, okVerdict, "stop")
	defer bad.Close()
	if _, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: bad.URL + "/v1", Model: "m", Backoff: time.Millisecond}).Judge(context.Background(), request()); err == nil {
		t.Fatal("400 must not be retried into success")
	}
	cut := chatServer(t, &got, nil, `{"passed":tr`, "length")
	defer cut.Close()
	if _, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: cut.URL + "/v1", Model: "m"}).Judge(context.Background(), request()); err == nil {
		t.Fatal("a truncated verdict must be an error")
	}
}

func TestOpenAICompatibleRequiresBaseURLAndDefaultsToAMiniModel(t *testing.T) {
	_, err := NewOpenAICompatible(Config{APIKey: "k"}).Judge(context.Background(), request())
	if err == nil || !strings.Contains(err.Error(), "JUDGE_BASE_URL") {
		t.Fatalf("no provider must be assumed: want a JUDGE_BASE_URL error, got %v", err)
	}
	var got captured
	srv := chatServer(t, &got, nil, okVerdict, "stop")
	defer srv.Close()
	if _, err := NewOpenAICompatible(Config{APIKey: "k", BaseURL: srv.URL + "/v1"}).Judge(context.Background(), request()); err != nil {
		t.Fatal(err)
	}
	if got.body["model"] != DefaultOpenAICompatibleModel || DefaultOpenAICompatibleModel != "gpt-5.4-mini" {
		t.Fatalf("default model = %v", got.body["model"])
	}
}
