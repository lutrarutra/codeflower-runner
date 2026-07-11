package compiler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatCompletionPreservesConversationRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := request.Header.Get("HTTP-Referer"); got != "codeflower.app" {
			t.Fatalf("HTTP-Referer = %q, want %q", got, "codeflower.app")
		}
		wantRoles := []string{"system", "user", "assistant", "user"}
		if len(body.Messages) != len(wantRoles) {
			t.Fatalf("message count = %d, want %d", len(body.Messages), len(wantRoles))
		}
		for index, wantRole := range wantRoles {
			if body.Messages[index].Role != wantRole {
				t.Errorf("message %d role = %q, want %q", index, body.Messages[index].Role, wantRole)
			}
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"[DONE]"}}]}`))
	}))
	defer server.Close()

	client := NewLLMClient(server.URL, "test-key", "test-model", map[string]string{"HTTP-Referer": "codeflower.app"})
	conversation := []chatMessage{
		{Role: "user", Content: "request help"},
		{Role: "assistant", Content: "[CMD: --help]"},
		{Role: "user", Content: "harness output"},
	}
	response, err := client.ChatCompletion("system", conversation)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if response != "[DONE]" {
		t.Fatalf("response = %q, want %q", response, "[DONE]")
	}
}
