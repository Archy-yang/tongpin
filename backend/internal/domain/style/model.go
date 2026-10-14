// Package style 风格画像域：异步分析对话生成风格卡、风格卡 CRUD、确定性向量化。
package style

import (
	"encoding/json"
	"time"
)

// SourceMessage 风格分析取数的消息源（通过消费方接口从 chat 域获取）。
type SourceMessage struct {
	ID        int64
	Sender    string // "user" | "ai"
	Content   string
	CreatedAt time.Time
}

// ---- 风格卡结构（与 docs/product/03-style-profile.md 的 JSON Schema 一一对应）----

type CommunicationStyle struct {
	Pace        string  `json:"pace"`         // fast|medium|slow
	ReplyLength string  `json:"reply_length"` // short|short_to_medium|medium|long
	Initiative  float64 `json:"initiative"`   // 0.0-1.0
	EmojiUsage  string  `json:"emoji_usage"`  // none|low|medium|high
	Formality   string  `json:"formality"`    // formal|neutral|casual
}

type EmotionalStyle struct {
	Expression         string `json:"expression"`          // direct|reserved
	Tone               string `json:"tone"`                // positive|calm|melancholic
	VulnerabilitySpeed string `json:"vulnerability_speed"` // fast|medium|slow
}

type Boundary struct {
	PrivacySensitivity   string `json:"privacy_sensitivity"`   // high|medium|low
	PhysicalContactSpeed string `json:"physical_contact_speed"` // fast|medium|slow
}

type StyleProfile struct {
	CommunicationStyle CommunicationStyle `json:"communication_style"`
	EmotionalStyle     EmotionalStyle     `json:"emotional_style"`
	TopicPreferences   []string           `json:"topic_preferences"`
	ConflictStyle      string             `json:"conflict_style"`       // avoidant|rational|emotional|compromising
	Boundary           Boundary           `json:"boundary"`
	RelationshipIntent string             `json:"relationship_intent"` // casual|friends|serious|serious_but_slow
	ActiveHours        []string           `json:"active_hours"`        // morning|afternoon|evening|late_night（代码计算，非 LLM）
	Confidence         float64            `json:"confidence"`
}

// ProfileView GET /style-profile 的响应。
type ProfileView struct {
	Status       string          `json:"status"` // collecting|ready
	MessageCount int             `json:"message_count,omitempty"`
	Profile      json.RawMessage `json:"profile,omitempty"` // 展示层：已合并用户编辑
	Confidence   float64         `json:"confidence,omitempty"`
	Version      int             `json:"version,omitempty"`
}

type UpdateDisplayRequest struct {
	DisplayOverrides json.RawMessage `json:"display_overrides" binding:"required"`
}

// ---- 枚举集合（校验与向量化共用）----

var (
	enumPace        = []string{"fast", "medium", "slow"}
	enumReplyLength = []string{"short", "short_to_medium", "medium", "long"}
	enumEmojiUsage  = []string{"none", "low", "medium", "high"}
	enumFormality   = []string{"formal", "neutral", "casual"}
	enumExpression  = []string{"direct", "reserved"}
	enumTone        = []string{"positive", "calm", "melancholic"}
	enumVulnSpeed   = []string{"fast", "medium", "slow"}
	enumConflict    = []string{"avoidant", "rational", "emotional", "compromising"}
	enumSensitivity = []string{"high", "medium", "low"}
	enumIntent      = []string{"casual", "friends", "serious", "serious_but_slow"}
)

// ---- 阈值常量 ----

const (
	// 分析触发：距上次分析的新消息数达到该值才进入分析
	triggerNewMessages = 20
	// 单次分析最多携带的消息条数（控制 token 成本）
	maxAnalyzedMessages = 100
	// 画像成熟线：总消息数达到该值才允许置信度超过 0.6
	matureMessageThreshold = 50
	confidenceCapImmature  = 0.6
)
