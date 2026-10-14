package style

import (
	"fmt"
	"strings"
)

// embeddingDim 匹配向量维度。必须与 migrations/0003_style.sql 的 VECTOR(64) 一致。
const embeddingDim = 64

// topicVocab 话题固定词表（20 维），与 system prompt 中列出的词表一致。
var topicVocab = [...]string{
	"life", "work", "career", "music", "movies",
	"books", "games", "sports", "travel", "food",
	"relationships", "emotions", "philosophy", "technology", "art",
	"health", "finance", "family", "study", "late_night_thoughts",
}

// 活跃时段桶（4 维），顺序固定。
var hourBuckets = [...]string{"morning", "afternoon", "evening", "late_night"}

// BuildEmbedding 从风格卡确定性计算匹配向量（不走 LLM）。
// 布局：枚举字段 one-hot、数值字段归一化、话题多标签、时段多标签；未填值整段为 0。
func BuildEmbedding(p StyleProfile, activeHours []string) [embeddingDim]float32 {
	var v [embeddingDim]float32
	c := p.CommunicationStyle
	e := p.EmotionalStyle

	seg := 0
	set := func(vals []string, val string) {
		for i, name := range vals {
			if name == val {
				v[seg+i] = 1
				break
			}
		}
		seg += len(vals)
	}

	set(enumPace, c.Pace)                              // 3
	set(enumReplyLength, c.ReplyLength)                // 4
	set(enumEmojiUsage, c.EmojiUsage)                  // 4
	set(enumFormality, c.Formality)                    // 3
	set(enumExpression, e.Expression)                  // 2
	set(enumTone, e.Tone)                              // 3
	set(enumVulnSpeed, e.VulnerabilitySpeed)           // 3
	set(enumConflict, p.ConflictStyle)                 // 4（枚举本身，无 unknown 位）
	set(enumSensitivity, p.Boundary.PrivacySensitivity) // 3
	set(enumVulnSpeed, p.Boundary.PhysicalContactSpeed) // 3
	set(enumIntent, p.RelationshipIntent)              // 4

	v[seg] = float32(p.CommunicationStyle.Initiative) // 1 归一化数值
	seg++

	for _, h := range activeHours { // 4 时段多标签
		for i, name := range hourBuckets {
			if name == h {
				v[seg+i] = 1
				break
			}
		}
	}
	seg += len(hourBuckets)

	for _, t := range p.TopicPreferences { // 20
		for i, name := range topicVocab {
			if name == t {
				v[seg+i] = 1
				break
			}
		}
	}

	return v
}

// embeddingLiteral 转 pgvector 字面量 "[0,1,...]"。
func embeddingLiteral(v [embeddingDim]float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%.4f", x)
	}
	b.WriteByte(']')
	return b.String()
}
