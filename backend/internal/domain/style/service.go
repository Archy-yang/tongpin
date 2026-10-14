package style

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// Completer 是本域对 LLM 的最小依赖：一次系统提示 + 一次用户提示的补全调用。
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// ChatMessageSource 是本域对 chat 域的最小依赖（消费方接口，装配层注入适配器）。
type ChatMessageSource interface {
	MessagesSince(ctx context.Context, userID, afterID int64, limit int) ([]SourceMessage, error)
	CountMessages(ctx context.Context, userID int64) (int, error)
}

type Service struct {
	repo    *Repository
	chat    ChatMessageSource
	completer Completer
}

func NewService(repo *Repository, chat ChatMessageSource, completer Completer) *Service {
	return &Service{repo: repo, chat: chat, completer: completer}
}

// AnalyzeUser 对单个用户做一次增量风格分析。由 Worker 调用，不在 HTTP 链路上。
func (s *Service) AnalyzeUser(ctx context.Context, userID int64) error {
	lastMsgID, _, err := s.repo.GetState(ctx, userID)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}

	msgs, err := s.chat.MessagesSince(ctx, userID, lastMsgID, maxAnalyzedMessages)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}
	if len(msgs) == 0 {
		return nil
	}
	// 本次分析覆盖到的最大消息 ID：仅在成功落库后推进（UpsertState），
	// 失败则下个周期自动重试同一批消息。
	newLast := msgs[len(msgs)-1].ID

	prev, _ := s.repo.LatestProfile(ctx, userID)
	var prevProfile *StyleProfile
	if prev != nil {
		var p StyleProfile
		if json.Unmarshal(prev.Profile, &p) == nil {
			prevProfile = &p
		}
	}

	raw, err := s.completer.Complete(ctx, analysisSystemPrompt, buildUserPrompt(prevProfile, msgs))
	if err != nil {
		return fmt.Errorf("llm analyze: %w", err)
	}

	profile, err := ParseProfile(raw)
	if err != nil {
		return fmt.Errorf("invalid analysis output: %w", err)
	}
	profile.ActiveHours = computeActiveHours(msgs)
	profile.Confidence = clamp01(profile.Confidence)

	total, err := s.chat.CountMessages(ctx, userID)
	if err != nil {
		return fmt.Errorf("count messages: %w", err)
	}
	if total < matureMessageThreshold && profile.Confidence > confidenceCapImmature {
		profile.Confidence = confidenceCapImmature
	}

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	nextVersion := 1
	if prev != nil {
		nextVersion = prev.Version + 1
	}
	if err := s.repo.InsertProfile(ctx, userID, nextVersion, profileJSON, profile.Confidence, BuildEmbedding(profile, profile.ActiveHours)); err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	if err := s.repo.UpsertState(ctx, userID, newLast); err != nil {
		return fmt.Errorf("upsert state: %w", err)
	}
	log.Printf("[style] analyzed user %d: version %d, confidence %.2f", userID, nextVersion, profile.Confidence)
	return nil
}

// View 风格卡查看：collecting（未生成）或 ready（含展示层合并结果）。
func (s *Service) View(ctx context.Context, userID int64) (ProfileView, error) {
	total, err := s.chat.CountMessages(ctx, userID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("count messages: %w", err)
	}

	rec, err := s.repo.LatestProfile(ctx, userID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("latest profile: %w", err)
	}
	if rec == nil {
		return ProfileView{Status: "collecting", MessageCount: total}, nil
	}

	overrides, err := s.repo.GetOverrides(ctx, userID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("get overrides: %w", err)
	}
	display := json.RawMessage(rec.Profile)
	if len(overrides) > 0 {
		if merged, ok := shallowMerge(rec.Profile, overrides); ok {
			display = merged
		}
	}
	return ProfileView{
		Status:       "ready",
		MessageCount: total,
		Profile:      display,
		Confidence:   rec.Confidence,
		Version:      rec.Version,
	}, nil
}

// UpdateDisplay 保存展示层编辑（不影响匹配用画像）。
func (s *Service) UpdateDisplay(ctx context.Context, userID int64, overrides json.RawMessage) error {
	if !json.Valid(overrides) {
		return fmt.Errorf("invalid json")
	}
	if len(overrides) > 8*1024 {
		return fmt.Errorf("overrides too large")
	}
	return s.repo.UpsertOverrides(ctx, userID, overrides)
}

// Delete 删除全部风格卡并停止分析任务（回到陪伴阶段）。
func (s *Service) Delete(ctx context.Context, userID int64) error {
	if err := s.repo.DeleteAllProfiles(ctx, userID); err != nil {
		return fmt.Errorf("delete profiles: %w", err)
	}
	return s.repo.SetSuppressed(ctx, userID, true)
}

// Resume 用户删除后重新开启分析。
func (s *Service) Resume(ctx context.Context, userID int64) error {
	return s.repo.SetSuppressed(ctx, userID, false)
}

// shallowMerge 展示层合并：overrides 顶层字段覆盖 profile 同名字段。
func shallowMerge(base, overrides []byte) (json.RawMessage, bool) {
	var bm, om map[string]json.RawMessage
	if json.Unmarshal(base, &bm) != nil || json.Unmarshal(overrides, &om) != nil {
		return nil, false
	}
	for k, v := range om {
		bm[k] = v // 浅合并：overrides 顶层字段覆盖 profile 同名字段
	}
	var buf bytes.Buffer
	if json.NewEncoder(&buf).Encode(bm) != nil {
		return nil, false
	}
	return json.RawMessage(buf.Bytes()), true
}

// computeActiveHours 从消息时间戳统计活跃时段（占比 >= 20% 的桶），不经 LLM。
func computeActiveHours(msgs []SourceMessage) []string {
	counts := map[string]int{}
	total := 0
	for _, m := range msgs {
		if m.Sender != "user" {
			continue
		}
		bucket := hourBuckets[bucketIndex(m.CreatedAt.Hour())]
		counts[bucket]++
		total++
	}
	if total == 0 {
		return nil
	}
	var out []string
	for _, b := range hourBuckets {
		if float64(counts[b])/float64(total) >= 0.2 {
			out = append(out, b)
		}
	}
	return out
}

func bucketIndex(hour int) int {
	switch {
	case hour >= 6 && hour < 12:
		return 0 // morning
	case hour >= 12 && hour < 18:
		return 1 // afternoon
	case hour >= 18:
		return 2 // evening
	default:
		return 3 // late_night (0-5)
	}
}
