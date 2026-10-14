package chat

import (
	"testing"
)

func TestBuildRequest(t *testing.T) {
	s := &Service{}
	history := []Message{
		{ID: 1, Sender: "user", Content: "你好"},
		{ID: 2, Sender: "ai", Content: "你好呀，今天过得怎么样？"},
		{ID: 3, Sender: "user", Content: "还行，刚下班"},
	}

	req := s.buildRequest(history)

	if len(req.Messages) != len(history)+1 {
		t.Fatalf("expect %d messages, got %d", len(history)+1, len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != personaPrompt {
		t.Errorf("first message must be system persona, got role=%s", req.Messages[0].Role)
	}
	cases := []struct {
		idx  int
		role string
	}{
		{1, "user"}, {2, "assistant"}, {3, "user"},
	}
	for _, c := range cases {
		if req.Messages[c.idx].Role != c.role {
			t.Errorf("message[%d] role = %s, want %s", c.idx, req.Messages[c.idx].Role, c.role)
		}
	}
	if req.Messages[3].Content != "还行，刚下班" {
		t.Errorf("last message content mismatch: %s", req.Messages[3].Content)
	}
	if req.Temperature != temperature || req.MaxTokens != maxTokens {
		t.Errorf("sampling params mismatch: temp=%f maxTokens=%d", req.Temperature, req.MaxTokens)
	}
}

func TestToDTOAIMarker(t *testing.T) {
	ai := toDTO(Message{ID: 1, Sender: "ai", Content: "hi"})
	if !ai.IsAI {
		t.Error("ai message must set is_ai = true")
	}
	user := toDTO(Message{ID: 2, Sender: "user", Content: "hi"})
	if user.IsAI {
		t.Error("user message must set is_ai = false")
	}
}
