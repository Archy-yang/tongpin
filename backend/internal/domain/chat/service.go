package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/Archy-yang/tongpin/backend/internal/llm"
)

const (
	// historyWindow 每次调用 LLM 携带的最近消息条数，控制 token 成本。
	historyWindow = 20
	maxTokens     = 500
	temperature   = 0.8

	// 长期记忆：距上次更新新增 ≥ 该条数才触发合并
	memoryTriggerNewMessages = 30
	// 单次记忆更新最多携带的消息条数
	maxMemoryMessages = 120
	// 记忆摘要长度上限（字符），控制注入成本
	memoryMaxChars = 800
)


// memorySystemPrompt 记忆整理器：维护跨会话的长期记忆摘要。
const memorySystemPrompt = `你是记忆整理器。根据对话记录，维护一份关于用户的长期记忆摘要，供 AI 陪伴者下次对话时自然使用。

规则：
- 只记录：兴趣偏好、生活近况、情绪基调、反复出现的话题、重要个人背景（如职业方向、所在城市这类粒度）、正在经历的事
- 不要记录：真实姓名、住址、证件号、财务状况、健康细节等敏感信息
- 用紧凑的中文短句列表，总长不超过 300 字
- 只输出摘要文本本身，不要任何解释或标题
- 若提供了旧摘要：在其基础上合并更新；旧内容没有新证据推翻就保留，别丢掉仍然有效的记忆`

// buildMemoryUserPrompt 组装记忆合并请求。
func buildMemoryUserPrompt(prev string, msgs []Message) string {
	var b strings.Builder
	if prev != "" {
		b.WriteString("当前的记忆摘要（在此基础上合并更新）：\n")
		b.WriteString(prev)
		b.WriteString("\n\n新对话记录：\n")
	} else {
		b.WriteString("对话记录（首次生成记忆）：\n")
	}
	for _, m := range msgs {
		tag := "user"
		if m.Sender != "user" {
			tag = "ai"
		}
		fmt.Fprintf(&b, "[%s] %s\n", tag, m.Content)
	}
	return b.String()
}

// memorySection 把记忆注入 system prompt 的段落。
func memorySection(summary string) string {
	if summary == "" {
		return ""
	}
	return "\n\n以下是你对这位用户的长期记忆（来自你们过去的相处）。自然地使用它——让对方感到被记得，但不要生硬复述：\n" + summary
}

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
	ReplyLength string   // short|short_to_medium|medium|long
	Formality   string   // formal|neutral|casual
	EmojiUsage  string   // none|low|medium|high
	Tone        string   // positive|calm|melancholic
	Pace        string   // fast|medium|slow
	ActiveHours []string // 用户活跃时段（主动开话的时段边界）
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
	notifier  AINotifier        // 可为 nil（主动消息不实时推送）
	quota     int               // 免费用户每日聊天条数；<=0 表示关闭额度限制
}

// ErrQuotaExceeded 当日免费额度用完（handler 据此返回 3002，引导升级会员）。
var ErrQuotaExceeded = errors.New("daily free quota exceeded")

func NewService(repo *Repository, completer ChatCompleter, hints StyleHintProvider) *Service {
	return &Service{repo: repo, completer: completer, hints: hints}
}

// SetFreeQuotaDefault 装配期注入全局默认免费额度（用户级 free_quota 覆盖它，见 checkQuota）。
func (s *Service) SetFreeQuotaDefault(n int) {
	s.quota = n
}

// SetStyleHints 装配期回填风格对齐依赖（chat 需要 style 的画像，style 又依赖 chat 的
// 消息，构造互依；此方法只在装配期、任何请求开始前调用一次，无并发问题）。
func (s *Service) SetStyleHints(hints StyleHintProvider) {
	s.hints = hints
}

// UpdateMemory 对单个会话做一次增量记忆合并（异步，不在聊天链路上）。
// 旧记忆作为合并基础；失败不推进覆盖位置，下个周期自动重试。
func (s *Service) UpdateMemory(ctx context.Context, conv Conversation) error {
	msgs, err := s.repo.MessagesAfter(ctx, conv.ID, conv.LastMemoryMsgID, maxMemoryMessages)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}
	if len(msgs) == 0 {
		return nil
	}

	raw, err := s.completer.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: memorySystemPrompt},
			{Role: "user", Content: buildMemoryUserPrompt(conv.MemorySummary, msgs)},
		},
		Temperature: 0.3, // 整理任务求稳
		MaxTokens:   400,
	})
	if err != nil {
		return fmt.Errorf("llm summarize: %w", err)
	}

	summary := strings.TrimSpace(raw)
	summary = strings.TrimPrefix(summary, "```")
	summary = strings.TrimSuffix(summary, "```")
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Errorf("empty memory summary")
	}
	if len([]rune(summary)) > memoryMaxChars {
		summary = string([]rune(summary)[:memoryMaxChars])
	}

	if err := s.repo.SaveMemory(ctx, conv.ID, summary, msgs[len(msgs)-1].ID); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}
	log.Printf("[chat] memory updated: conv %d (user %d), %d new messages", conv.ID, conv.UserID, len(msgs))
	return nil
}

// SendMessage 保存用户消息 → 携带上下文调用 LLM → 保存并返回 AI 回复。
// 用户消息先落库：即使 LLM 失败，对话记录不丢。
func (s *Service) SendMessage(ctx context.Context, userID int64, content string) (SendMessageResponse, error) {
	// 额度检查在落库之前：超额时用户消息不产生半条记录，由前端把草稿还回输入框
	if err := s.checkQuota(ctx, userID); err != nil {
		return SendMessageResponse{}, err
	}

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

	reply, err := s.completer.Chat(ctx, s.buildRequest(history, hint, conv.MemorySummary))
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("llm complete: %w", err)
	}

	aiMsg, err := s.repo.InsertMessage(ctx, conv.ID, "ai", reply)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("save ai message: %w", err)
	}

	return SendMessageResponse{UserMessage: toDTO(userMsg), AIMessage: toDTO(aiMsg)}, nil
}

// checkQuota 免费额度边界：用户级 free_quota 覆盖全局默认（0=用默认，负数=不限额/会员占位）。
// 检查在用户消息落库前做，超额直接拒绝——不产生半截对话记录。
func (s *Service) checkQuota(ctx context.Context, userID int64) error {
	if s.quota <= 0 {
		return nil // 全局关闭（测试模式 / 显式不限制）
	}
	limit, err := s.repo.UserQuota(ctx, userID)
	if err != nil {
		return fmt.Errorf("load quota: %w", err)
	}
	if limit < 0 {
		return nil // 不限额（会员占位）
	}
	if limit == 0 {
		limit = s.quota
	}
	sent, err := s.repo.CountMessagesToday(ctx, userID)
	if err != nil {
		return fmt.Errorf("count today: %w", err)
	}
	if sent >= limit {
		return ErrQuotaExceeded
	}
	return nil
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

// buildRequest 组装 system 人格（长期记忆 + 风格对齐）+ 最近历史。
// 历史中已包含刚落库的用户消息。
// 画像对齐与即时镜像互斥：对齐生效时以对齐为准（更了解用户，且避免两个规则打架）；
// 无画像或置信度不足时退回即时镜像（从第 1 句起就有同频感）。
func (s *Service) buildRequest(history []Message, hint *StyleHint, memory string) llm.ChatRequest {
	style := styleAlignment(hint)
	if style == "" {
		style = mirrorRule
	}
	messages := make([]llm.Message, 0, len(history)+1)
	messages = append(messages, llm.Message{Role: "system", Content: personaBase + memorySection(memory) + style})
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
