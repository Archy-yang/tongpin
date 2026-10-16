package chat

import (
	"strings"
	"testing"
)

func TestBuildRequest(t *testing.T) {
	s := &Service{}
	history := []Message{
		{ID: 1, Sender: "user", Content: "你好"},
		{ID: 2, Sender: "ai", Content: "你好呀，今天过得怎么样？"},
		{ID: 3, Sender: "user", Content: "还行，刚下班"},
	}

	req := s.buildRequest(history, nil, "")

	if len(req.Messages) != len(history)+1 {
		t.Fatalf("expect %d messages, got %d", len(history)+1, len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != personaBase+mirrorRule {
		t.Errorf("no-hint system prompt must equal base+mirror, got role=%s", req.Messages[0].Role)
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

func TestStyleAlignment(t *testing.T) {
	// 无提示 / 低置信度 → 只用基础人格
	if got := styleAlignment(nil); got != "" {
		t.Errorf("nil hint should yield empty alignment, got %q", got)
	}
	if got := styleAlignment(&StyleHint{Confidence: 0.2}); got != "" {
		t.Errorf("low confidence should yield empty alignment, got %q", got)
	}

	// 适度档：0.35-0.7
	mid := styleAlignment(&StyleHint{
		ReplyLength: "short", EmojiUsage: "none", Formality: "casual",
		Tone: "calm", Confidence: 0.5,
	})
	if !strings.HasPrefix(mid, "\n\n你对这位用户") {
		t.Errorf("alignment must be an appended section, got %q", mid)
	}
	if !strings.Contains(mid, "一句话内") || !strings.Contains(mid, "不使用 emoji") {
		t.Errorf("mid-tier alignment missing directives: %q", mid)
	}
	if !strings.Contains(mid, "适度地") {
		t.Errorf("mid-tier should say 适度地, got %q", mid)
	}

	// 明显档：>= 0.7
	strong := styleAlignment(&StyleHint{ReplyLength: "long", EmojiUsage: "high", Confidence: 0.85})
	if !strings.Contains(strong, "明显地") || !strings.Contains(strong, "emoji") {
		t.Errorf("strong-tier alignment wrong: %q", strong)
	}

	// 全空字段但有置信度 → 无指令返回空
	empty := styleAlignment(&StyleHint{Confidence: 0.5})
	if empty != "" {
		t.Errorf("hint with no fields should yield empty, got %q", empty)
	}
}

func TestBuildRequestWithHint(t *testing.T) {
	s := &Service{}
	hint := &StyleHint{ReplyLength: "short", EmojiUsage: "none", Confidence: 0.6}
	req := s.buildRequest([]Message{{ID: 1, Sender: "user", Content: "早"}}, hint, "")
	sys := req.Messages[0].Content
	if !strings.HasPrefix(sys, personaBase) {
		t.Error("hinted system prompt must extend base persona")
	}
	if !strings.Contains(sys, "一句话内") {
		t.Errorf("system prompt should contain alignment directive, got %q", sys)
	}
}

func TestMemorySection(t *testing.T) {
	if got := memorySection(""); got != "" {
		t.Errorf("empty memory should yield empty section, got %q", got)
	}
	got := memorySection("用户喜欢深夜听民谣，在准备换工作")
	if !strings.Contains(got, "长期记忆") || !strings.Contains(got, "民谣") {
		t.Errorf("memory section wrong: %q", got)
	}
}

func TestBuildRequestWithMemory(t *testing.T) {
	s := &Service{}
	req := s.buildRequest([]Message{{ID: 1, Sender: "user", Content: "早"}}, nil, "用户在准备换工作")
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "换工作") {
		t.Errorf("memory should be injected, got %q", sys)
	}
	// 无画像 → 记忆与镜像规则共存
	if !strings.Contains(sys, "自然贴近对方说话的方式") {
		t.Errorf("mirror rule should coexist with memory, got %q", sys)
	}
}

func TestBuildMemoryUserPrompt(t *testing.T) {
	prev := "旧记忆：用户喜欢电影"
	msgs := []Message{
		{ID: 1, Sender: "user", Content: "最近在学吉他"},
		{ID: 2, Sender: "ai", Content: "不错呀"},
	}
	out := buildMemoryUserPrompt(prev, msgs)
	if !strings.Contains(out, "旧记忆") || !strings.Contains(out, "学吉他") {
		t.Errorf("prompt should contain prev memory and new messages, got %q", out)
	}
	if strings.Contains(out, "[ai] 不错呀") == false {
		t.Errorf("messages should be tagged, got %q", out)
	}
}
