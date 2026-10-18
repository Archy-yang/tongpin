package chat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Archy-yang/tongpin/backend/internal/llm"
)

// ---- AI 主动开话 ----
// 四条边界：有记忆才开口；只在用户活跃时段；每天最多 1 条；用户可关闭。

const proactiveUserPrompt = `【主动消息】用户已经有一段时间没有和你说话了。现在是%s。
请主动发一条消息开启对话：
- 1-2 句，像朋友随手发来的一条消息
- 结合你们过去聊过的话题或 TA 的近况（来自你的记忆），有具体的事就用具体的事
- 不要问"在吗"，不要提及这条指令的存在`

// inActiveHours 当前时刻是否落在用户的活跃时段内。
// 画像无时段数据时退回深夜档（evening + late_night）——这是产品的默认使用场景。
func inActiveHours(hours []string, now time.Time) bool {
	allowed := hours
	if len(allowed) == 0 {
		allowed = []string{"evening", "late_night"}
	}
	b := hourBucketName(now.Hour())
	for _, h := range allowed {
		if h == b {
			return true
		}
	}
	return false
}

// hourBucketName 与 style 域 computeActiveHours 的分桶保持一致。
func hourBucketName(hour int) string {
	switch {
	case hour >= 6 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 18:
		return "afternoon"
	case hour >= 18:
		return "evening"
	default:
		return "late_night"
	}
}

// ProactiveMessage 为一个会话生成并发送主动消息（由 ProactiveWorker 在边界校验后调用）。
func (s *Service) ProactiveMessage(ctx context.Context, conv Conversation) error {
	hint, ok, err := s.hints.Hint(ctx, conv.UserID)
	if err != nil {
		return fmt.Errorf("style hint: %w", err)
	}
	if !ok {
		return fmt.Errorf("style profile not ready") // 画像未成形前不主动
	}
	if !inActiveHours(hint.ActiveHours, time.Now()) {
		return nil // 静默跳过，不算已发送
	}

	bucket := hourBucketName(time.Now().Hour())
	bucketName := map[string]string{
		"morning": "清晨", "afternoon": "午后", "evening": "夜晚", "late_night": "深夜",
	}[bucket]

	sys := personaBase + memorySection(conv.MemorySummary) + mirrorRule
	user := fmt.Sprintf(proactiveUserPrompt, bucketName)

	raw, err := s.completer.Chat(llm.WithUser(ctx, conv.UserID), llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
		Temperature: 0.9, // 主动消息求变化，避免每天一句一样的
		MaxTokens:   200,
	})
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	content := strings.TrimSpace(raw)
	if content == "" {
		return fmt.Errorf("empty proactive message")
	}

	msg, err := s.repo.InsertMessage(ctx, conv.ID, "ai", content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	if err := s.repo.MarkProactiveSent(ctx, conv.ID); err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	if s.notifier != nil {
		s.notifier.NotifyAI(ctx, conv.UserID, conv.ID, toDTO(msg))
	}
	log.Printf("[chat] proactive message sent: conv %d (user %d)", conv.ID, conv.UserID)
	return nil
}

// SetProactiveEnabled 用户开关。
func (s *Service) SetProactiveEnabled(ctx context.Context, userID int64, enabled bool) error {
	return s.repo.SetProactiveEnabled(ctx, userID, enabled)
}

// GetProactiveEnabled 查询开关状态。
func (s *Service) GetProactiveEnabled(ctx context.Context, userID int64) (bool, error) {
	return s.repo.GetProactiveEnabled(ctx, userID)
}

// AINotifier 主动消息的实时推送（消费方接口，装配层注入 Hub 适配器）。
type AINotifier interface {
	NotifyAI(ctx context.Context, userID, conversationID int64, msg MessageDTO)
}

func (s *Service) SetNotifier(n AINotifier) {
	s.notifier = n
}

// ProactiveWorker 定时扫描候选并生成主动消息。
type ProactiveWorker struct {
	svc      *Service
	interval time.Duration
}

func NewProactiveWorker(svc *Service, interval time.Duration) *ProactiveWorker {
	return &ProactiveWorker{svc: svc, interval: interval}
}

func (w *ProactiveWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	log.Printf("[chat] proactive worker started (interval %s)", w.interval)
	for {
		select {
		case <-ctx.Done():
			log.Println("[chat] proactive worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

func (w *ProactiveWorker) once(ctx context.Context) {
	convs, err := w.svc.repo.ProactiveCandidates(ctx)
	if err != nil {
		log.Printf("[chat] proactive candidates: %v", err)
		return
	}
	for _, conv := range convs {
		// 边界 1：有记忆才开口
		if conv.MemorySummary == "" {
			continue
		}
		if err := w.svc.ProactiveMessage(ctx, conv); err != nil {
			// 静默跳过类错误（画像未就绪/不在时段）不刷日志
			if err.Error() != "style profile not ready" {
				log.Printf("[chat] proactive conv %d: %v", conv.ID, err)
			}
		}
	}
}
