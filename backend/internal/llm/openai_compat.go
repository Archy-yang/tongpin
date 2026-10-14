// Package llm 提供 LLM 调用抽象。统一走 OpenAI 兼容协议，
// DeepSeek / GLM / Kimi / Claude(兼容网关) 等均可通过配置切换。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout = 60 * time.Second
	defaultModel   = "deepseek-chat"
)

// Message 是一次对话中的单条消息。Role 取 "system" | "user" | "assistant"。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 描述一次补全调用。
type ChatRequest struct {
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

// Provider 是所有 LLM 实现的统一接口。业务域只依赖此接口。
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (string, error)
}

// OpenAICompat 调用 OpenAI 兼容的 /chat/completions 接口。
type OpenAICompat struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	if model == "" {
		model = defaultModel
	}
	return &OpenAICompat{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAICompat) Chat(ctx context.Context, req ChatRequest) (string, error) {
	payload, err := json.Marshal(chatCompletionRequest{
		Model:       p.model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call llm: %w", err)
	}
	defer resp.Body.Close()

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", fmt.Errorf("decode llm response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := "unknown error"
		if completion.Error != nil {
			msg = completion.Error.Message
		}
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, msg)
	}
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm returned empty completion")
	}
	return completion.Choices[0].Message.Content, nil
}
