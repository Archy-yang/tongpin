package chat

import (
	"context"
	"fmt"
	"math"

	"github.com/Archy-yang/tongpin/backend/internal/llm"
)

const (
	// historyWindow 每次调用 LLM 携带的最近消息条数，控制 token 成本。
	historyWindow = 20
	maxTokens     = 500
	temperature   = 0.8
)

// personaPrompt AI 基础人格。系统级设定，与对话历史分离。
const personaPrompt = `你是「同频」的 AI 陪伴者。你的性格温暖、真诚、有好奇心，善于倾听，不说教、不敷衍。
你正在与一位用户日常对话，目标是陪伴与理解，并在自然交流中了解对方的沟通风格与兴趣。

要求：
- 回复口语化、自然，通常 1-3 句话，不要长篇大论
- 像朋友一样回应，可以适当提问让对话继续
- 你是 AI，不是真人；如果用户询问你的身份，如实回答
- 用户表达强烈负面情绪时先共情，必要时温和建议寻求现实支持
- 遇到涉及自伤或危害他人的内容，认真回应并建议联系专业援助`

// ChatCompleter 是本域对 LLM 能力的最小依赖（消费方定义接口，装配层注入）。
type ChatCompleter interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

type Service struct {
	repo    *Repository
	completer ChatCompleter
}

func NewService(repo *Repository, completer ChatCompleter) *Service {
	return &Service{repo: repo, completer: completer}
}

// SendMessage 保存用户消息 → 携带上下文调用 LLM → 保存并返回 AI 回复。
// 用户消息先落库：即使 LLM 失败，对话记录不丢。
func (s *Service) SendMessage(ctx context.Context, userID int64, content string) (SendMessageResponse, error) {
	conv, err := s.repo.GetOrCreateConversation(ctx, userID)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("get conversation: %w", err)
	}

	userMsg, err := s.repo.InsertMessage(ctx, conv.ID, "user", content)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("save user message: %w", err)
	}

	history, err := s.repo.RecentMessages(ctx, conv.ID, historyWindow)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("load history: %w", err)
	}

	reply, err := s.completer.Chat(ctx, s.buildRequest(history))
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("llm complete: %w", err)
	}

	aiMsg, err := s.repo.InsertMessage(ctx, conv.ID, "ai", reply)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("save ai message: %w", err)
	}

	return SendMessageResponse{UserMessage: toDTO(userMsg), AIMessage: toDTO(aiMsg)}, nil
}

// History 游标分页拉取历史消息。before = 0 表示从最新开始。
func (s *Service) History(ctx context.Context, userID, before int64, limit int) ([]MessageDTO, error) {
	if before == 0 {
		before = math.MaxInt64
	}
	conv, err := s.repo.GetOrCreateConversation(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	msgs, err := s.repo.ListBefore(ctx, conv.ID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	out := make([]MessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toDTO(m))
	}
	return out, nil
}

// MessagesSince 供其他域（style）按消费方接口取数：某用户 afterID 之后的消息，正序。
func (s *Service) MessagesSince(ctx context.Context, userID, afterID int64, limit int) ([]Message, error) {
	return s.repo.MessagesSinceByUser(ctx, userID, afterID, limit)
}

// CountMessages 某用户的消息总数。
func (s *Service) CountMessages(ctx context.Context, userID int64) (int, error) {
	return s.repo.CountByUser(ctx, userID)
}

// buildRequest 组装 system 人格 + 最近历史。历史中已包含刚落库的用户消息。
func (s *Service) buildRequest(history []Message) llm.ChatRequest {
	messages := make([]llm.Message, 0, len(history)+1)
	messages = append(messages, llm.Message{Role: "system", Content: personaPrompt})
	for _, m := range history {
		role := "user"
		if m.Sender == "ai" {
			role = "assistant"
		}
		messages = append(messages, llm.Message{Role: role, Content: m.Content})
	}
	return llm.ChatRequest{
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
}
