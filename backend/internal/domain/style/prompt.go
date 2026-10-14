package style

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// analysisSystemPrompt 风格分析的系统提示。与 docs/product/03-style-profile.md 保持同步。
const analysisSystemPrompt = `你是风格分析器。根据对话记录，分析用户的沟通风格，输出严格 JSON。
规则：
- 不要推断敏感个人信息（真实姓名、地址、身份证、财务、健康）
- 只分析沟通风格、情绪表达、话题偏好、边界感、关系意图
- active_hours 由系统另行计算，你的输出中该字段固定为 []
- 证据不足的字段输出 null 或空数组，不要猜测
- confidence 是你对本次分析的整体把握，0.0-1.0
- 只输出 JSON，不要任何解释文字或代码块标记

输出格式：
{
  "communication_style": {
    "pace": "fast|medium|slow",
    "reply_length": "short|short_to_medium|medium|long",
    "initiative": 0.0-1.0,
    "emoji_usage": "none|low|medium|high",
    "formality": "formal|neutral|casual"
  },
  "emotional_style": {
    "expression": "direct|reserved",
    "tone": "positive|calm|melancholic",
    "vulnerability_speed": "fast|medium|slow"
  },
  "topic_preferences": ["life|work|career|music|movies|books|games|sports|travel|food|relationships|emotions|philosophy|technology|art|health|finance|family|study|late_night_thoughts 中的子集"],
  "conflict_style": "avoidant|rational|emotional|compromising 或 null",
  "boundary": {
    "privacy_sensitivity": "high|medium|low",
    "physical_contact_speed": "fast|medium|slow"
  },
  "relationship_intent": "casual|friends|serious|serious_but_slow 或 null",
  "active_hours": [],
  "confidence": 0.0-1.0
}`

// buildUserPrompt 组装分析请求：增量更新指令 + 历史风格卡 + 本次对话记录。
func buildUserPrompt(prev *StyleProfile, msgs []SourceMessage) string {
	var b strings.Builder
	b.WriteString("对话记录（[user] 是待分析用户，[ai] 是 AI 陪伴者，不计入用户风格）：\n\n")
	for _, m := range msgs {
		tag := "user"
		if m.Sender != "user" {
			tag = "ai"
		}
		fmt.Fprintf(&b, "[%s] %s\n", tag, m.Content)
	}
	if prev != nil {
		if prevJSON, err := json.Marshal(prev); err == nil {
			b.WriteString("\n已有一份历史风格卡，本次为增量更新，输出合并后的完整风格卡：\n")
			b.Write(prevJSON)
		}
	}
	return b.String()
}

// ParseProfile 解析并校验 LLM 输出的风格卡。未知字段被丢弃（结构体反序列化天然白名单）。
func ParseProfile(raw string) (StyleProfile, error) {
	var p StyleProfile
	if err := json.Unmarshal([]byte(stripFences(raw)), &p); err != nil {
		return StyleProfile{}, fmt.Errorf("parse style profile: %w", err)
	}
	if err := validateProfile(&p); err != nil {
		return StyleProfile{}, err
	}
	return p, nil
}

// stripFences 兼容 LLM 用 ```json ... ``` 包裹输出的情况。
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	return strings.TrimSuffix(strings.TrimSpace(s), "```")
}

// validateProfile 枚举字段合法性校验；非法值直接拒绝（宁可少一次更新，不留脏数据）。
func validateProfile(p *StyleProfile) error {
	c := &p.CommunicationStyle
	if !inEnumOrEmpty(c.Pace, enumPace) {
		return fmt.Errorf("invalid pace: %q", c.Pace)
	}
	if !inEnumOrEmpty(c.ReplyLength, enumReplyLength) {
		return fmt.Errorf("invalid reply_length: %q", c.ReplyLength)
	}
	if !inEnumOrEmpty(c.EmojiUsage, enumEmojiUsage) {
		return fmt.Errorf("invalid emoji_usage: %q", c.EmojiUsage)
	}
	if !inEnumOrEmpty(c.Formality, enumFormality) {
		return fmt.Errorf("invalid formality: %q", c.Formality)
	}
	c.Initiative = clamp01(c.Initiative)

	e := &p.EmotionalStyle
	if !inEnumOrEmpty(e.Expression, enumExpression) {
		return fmt.Errorf("invalid expression: %q", e.Expression)
	}
	if !inEnumOrEmpty(e.Tone, enumTone) {
		return fmt.Errorf("invalid tone: %q", e.Tone)
	}
	if !inEnumOrEmpty(e.VulnerabilitySpeed, enumVulnSpeed) {
		return fmt.Errorf("invalid vulnerability_speed: %q", e.VulnerabilitySpeed)
	}

	if !inEnumOrEmpty(p.ConflictStyle, enumConflict) {
		return fmt.Errorf("invalid conflict_style: %q", p.ConflictStyle)
	}
	if !inEnumOrEmpty(p.Boundary.PrivacySensitivity, enumSensitivity) {
		return fmt.Errorf("invalid privacy_sensitivity: %q", p.Boundary.PrivacySensitivity)
	}
	if !inEnumOrEmpty(p.Boundary.PhysicalContactSpeed, enumVulnSpeed) {
		return fmt.Errorf("invalid physical_contact_speed: %q", p.Boundary.PhysicalContactSpeed)
	}
	if !inEnumOrEmpty(p.RelationshipIntent, enumIntent) {
		return fmt.Errorf("invalid relationship_intent: %q", p.RelationshipIntent)
	}

	// 话题白名单过滤
	if len(p.TopicPreferences) > 0 {
		allowed := make(map[string]bool, len(topicVocab))
		for _, t := range topicVocab {
			allowed[t] = true
		}
		filtered := p.TopicPreferences[:0]
		for _, t := range p.TopicPreferences {
			if allowed[t] {
				filtered = append(filtered, t)
			}
		}
		p.TopicPreferences = filtered
	}
	return nil
}

func inEnumOrEmpty(v string, enum []string) bool {
	return v == "" || slices.Contains(enum, v)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
