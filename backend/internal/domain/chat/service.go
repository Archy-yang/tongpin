package chat

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Archy-yang/tongpin/backend/internal/llm"
)

const (
	// historyWindow 每次调用 LLM 携带的最近消息条数，控制 token 成本。
	historyWindow = 20
	maxTokens     = 500
	temperature   = 0.8
)

// personaBase AI 基础人格（不含风格镜像规则——镜像与画像对齐互斥，见 styleAlignment）。
const personaBase = `你是「同频」的 AI 陪伴者。你的性格温暖、真诚、有好奇心，善于倾听，不说教、不敷衍。
你正在与一位用户日常对话，目标是陪伴与理解，并在自然交流中了解对方的沟通风格与兴趣。

要求：
- 回复口语化、自然，通常 1-3 句话，不要长篇大论
- 像朋友一样回应，可以适当提问让对话继续
- 你是 AI，不是真人；如果用户询问你的身份，如实回答
- 用户表达强烈负面情绪时先共情，必要时温和建议寻求现实支持
- 遇到涉及自伤或危害他人的内容，认真回应并建议联系专业援助`

// mirrorRule 即时镜像：无画像时生效（从第 1 句开始就有"同频感"）。
const mirrorRule = `
- 自然贴近对方说话的方式：对方话长你就多说两句，对方话短你就简短；对方用什么表情你就用同类表情，对方不用你就不用。贴近但不要夸张模仿，不要复读对方的用词`

// personaPrompt 无画像时使用的基础人格。
const personaPrompt = personaBase + mirrorRule

// StyleHint 风格对齐提示：来自用户风格卡（由装配层从 style 域适配而来）。
type StyleHint struct {
	ReplyLength string // short|short_to_medium|medium|long
	Formality   string // formal|neutral|casual
	EmojiUsage  string // none|low|medium|high
	Tone        string // positive|calm|melancholic
	Pace        string // fast|medium|slow
	Confidence  float64
}

// StyleHintProvider 本域对 style 域的最小依赖（消费方接口，装配层注入）。
type StyleHintProvider interface {
	Hint(ctx context.Context, userID int64) (StyleHint, bool, error)
}

// ChatCompleter 是本域对 LLM 能力的最小依赖（消费方定义接口，装配层注入）。
type ChatCompleter interface {
	Chat(ctx context.Context, req llm.ChatRequest) (string, error)
}

type Service struct {
	repo      *Repository
	completer ChatCompleter
	hints     StyleHintProvider // 可为 nil（无画像时纯基础人格）
}

func NewService(repo *Repository, completer ChatCompleter, hints StyleHintProvider) *Service {
	return &Service{repo: repo, completer: completer, hints: hints}
}

// SetStyleHints 装配期回填风格对齐依赖（chat 需要 style 的画像，style 又依赖 chat 的
// 消息，构造互依；此方法只在装配期、任何请求开始前调用一次，无并发问题）。
func (s *Service) SetStyleHints(hints StyleHintProvider) {
	s.hints = hints
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

	// 风格对齐：画像随分析版本逐步成熟，AI 的说话方式随之逐步对齐用户。
	// 取不到画像时静默降级为基础人格（即时镜像规则始终生效）。
	var hint *StyleHint
	if s.hints != nil {
		if h, ok, err := s.hints.Hint(ctx, userID); err == nil && ok {
			hint = &h
		}
	}

	reply, err := s.completer.Chat(ctx, s.buildRequest(history, hint))
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

// buildRequest 组装 system 人格（含风格对齐）+ 最近历史。历史中已包含刚落库的用户消息。
// 画像对齐与即时镜像互斥：对齐生效时以对齐为准（更了解用户，且避免两个规则打架）；
// 无画像或置信度不足时退回即时镜像（从第 1 句起就有同频感）。
func (s *Service) buildRequest(history []Message, hint *StyleHint) llm.ChatRequest {
	style := styleAlignment(hint)
	if style == "" {
		style = mirrorRule
	}
	messages := make([]llm.Message, 0, len(history)+1)
	messages = append(messages, llm.Message{Role: "system", Content: personaBase + style})
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

// styleAlignment 依据风格卡置信度分档注入对齐指令——画像越成熟，对齐越明显。
// 置信度 < 0.35 时返回空（只靠基础人格里的即时镜像规则）。
func styleAlignment(h *StyleHint) string {
	if h == nil || h.Confidence < 0.35 {
		return ""
	}

	strength := "适度地"
	if h.Confidence >= 0.7 {
		strength = "明显地"
	}

	var lines []string
	switch h.ReplyLength {
	case "short":
		lines = append(lines, "- 回复克制在一句话内")
	case "short_to_medium":
		lines = append(lines, "- 回复一到两句")
	case "medium":
		lines = append(lines, "- 回复两三句，可以展开一点")
	case "long":
		lines = append(lines, "- 可以充分展开，和对方聊得深入")
	}
	switch h.EmojiUsage {
	case "none":
		lines = append(lines, "- 不使用 emoji")
	case "low":
		lines = append(lines, "- 几乎不用 emoji，偶尔一个即可")
	case "medium":
		lines = append(lines, "- 每条回复自然地带上一个 emoji（如 🙂😅🎵）")
	case "high":
		lines = append(lines, "- 每条回复带一两个 emoji（如 😂🎵🥲），语气活泼")
	}
	switch h.Formality {
	case "formal":
		lines = append(lines, "- 用词讲究一些，避免过于随意")
	case "casual":
		lines = append(lines, "- 完全口语化，像微信聊天")
	}
	switch h.Tone {
	case "calm":
		lines = append(lines, "- 语气平和低缓，不刻意热情")
	case "positive":
		lines = append(lines, "- 语气轻快明亮")
	case "melancholic":
		lines = append(lines, "- 语气沉静温柔，接得住低落")
	}
	switch h.Pace {
	case "slow":
		lines = append(lines, "- 不催促、多留白，允许话题慢慢漂移")
	case "fast":
		lines = append(lines, "- 节奏轻快，接话利落")
	}
	if len(lines) == 0 {
		return ""
	}

	body := strings.Join(lines, "\n")
	return fmt.Sprintf(
		"\n\n你对这位用户已经有了不少了解（把握约 %d%%）。请%s按下面的方式说话（这些指令优先于你对消息表面风格的临场模仿）：\n%s",
		int(h.Confidence*100), strength, body)
}
