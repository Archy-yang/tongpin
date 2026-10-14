package style

import (
	"testing"
	"time"
)

func TestBuildEmbeddingDim(t *testing.T) {
	p := StyleProfile{}
	v := BuildEmbedding(p, nil)
	if len(v) != embeddingDim {
		t.Fatalf("embedding dim = %d, want %d", len(v), embeddingDim)
	}
	// 空画像 → 全零向量
	allZero := true
	for _, x := range v {
		if x != 0 {
			allZero = false
		}
	}
	if !allZero {
		t.Error("empty profile should produce zero vector")
	}
}

func TestBuildEmbeddingDeterministic(t *testing.T) {
	p := StyleProfile{
		CommunicationStyle: CommunicationStyle{Pace: "slow", ReplyLength: "medium", Initiative: 0.7, EmojiUsage: "low", Formality: "casual"},
		EmotionalStyle:     EmotionalStyle{Expression: "reserved", Tone: "calm", VulnerabilitySpeed: "slow"},
		TopicPreferences:   []string{"music", "late_night_thoughts"},
		ConflictStyle:      "rational",
		Boundary:           Boundary{PrivacySensitivity: "high", PhysicalContactSpeed: "slow"},
		RelationshipIntent: "serious_but_slow",
	}
	hours := []string{"evening", "late_night"}

	v1 := BuildEmbedding(p, hours)
	v2 := BuildEmbedding(p, hours)
	if v1 != v2 {
		t.Fatal("same profile must produce identical vector")
	}

	// 校验几个关键位置：pace=slow 是 enumPace 第 3 位（索引 2）
	if v1[2] != 1 {
		t.Errorf("pace=slow should light index 2, got %v", v1[2])
	}
	// initiative=0.7 位于 one-hot 段之后：3+4+4+3+2+3+3+4+3+3+4 = 36
	if v1[36] < 0.69 || v1[36] > 0.71 {
		t.Errorf("initiative slot (36) = %v, want ~0.7", v1[36])
	}
	// 话题 music 应在最后 20 维段内：36+1+4 = 41 起是话题段，music 是 topicVocab[3] → 44
	if v1[44] != 1 {
		t.Errorf("topic 'music' should light index 44, got %v", v1[44])
	}
}

func TestParseProfileValid(t *testing.T) {
	raw := `{
		"communication_style": {"pace":"medium","reply_length":"short_to_medium","initiative":0.62,"emoji_usage":"low","formality":"casual"},
		"emotional_style": {"expression":"reserved","tone":"calm","vulnerability_speed":"slow"},
		"topic_preferences": ["music","movies","hacking"],
		"conflict_style": "rational",
		"boundary": {"privacy_sensitivity":"high","physical_contact_speed":"slow"},
		"relationship_intent": "serious_but_slow",
		"active_hours": ["22:00-01:00"],
		"confidence": 0.9,
		"injected_field": "ignore me"
	}`
	p, err := ParseProfile(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.CommunicationStyle.Pace != "medium" {
		t.Errorf("pace = %q", p.CommunicationStyle.Pace)
	}
	if p.Confidence != 0.9 {
		t.Errorf("confidence = %v", p.Confidence)
	}
	// 白名单外话题被过滤
	for _, topic := range p.TopicPreferences {
		if topic == "hacking" {
			t.Error("non-whitelisted topic must be filtered out")
		}
	}
}

func TestParseProfileFenced(t *testing.T) {
	raw := "```json\n{\"communication_style\":{\"pace\":\"fast\"},\"confidence\":0.5}\n```"
	p, err := ParseProfile(raw)
	if err != nil {
		t.Fatalf("fenced json should parse: %v", err)
	}
	if p.CommunicationStyle.Pace != "fast" {
		t.Errorf("pace = %q", p.CommunicationStyle.Pace)
	}
}

func TestParseProfileInvalidEnum(t *testing.T) {
	raw := `{"communication_style":{"pace":"lightning"},"confidence":0.5}`
	if _, err := ParseProfile(raw); err == nil {
		t.Fatal("invalid enum value must be rejected")
	}
}

func TestParseProfileGarbage(t *testing.T) {
	if _, err := ParseProfile("这不是 JSON"); err == nil {
		t.Fatal("non-json output must be rejected")
	}
}

func TestComputeActiveHours(t *testing.T) {
	mk := func(hour int) SourceMessage {
		return SourceMessage{Sender: "user", CreatedAt: time.Date(2026, 10, 14, hour, 0, 0, 0, time.Local)}
	}
	msgs := []SourceMessage{mk(23), mk(0), mk(1), mk(9), mk(14)}
	hours := computeActiveHours(msgs)
	// user 消息 5 条：23/0/1 → evening+late_night 各 1+2... 23→evening, 0,1→late_night(2), 9→morning(1), 14→afternoon(1)
	// 占比 >= 20% 的桶：late_night 2/5=40%，其余各 20%
	found := map[string]bool{}
	for _, h := range hours {
		found[h] = true
	}
	if !found["late_night"] || !found["morning"] || !found["evening"] || !found["afternoon"] {
		t.Errorf("active hours = %v, want all four (each >= 20%%)", hours)
	}

	// AI 消息不计入
	aiOnly := []SourceMessage{{Sender: "ai", CreatedAt: time.Date(2026, 10, 14, 3, 0, 0, 0, time.Local)}}
	if h := computeActiveHours(aiOnly); h != nil {
		t.Errorf("ai messages must not count, got %v", h)
	}
}

func TestEmbeddingLiteral(t *testing.T) {
	var v [embeddingDim]float32
	v[0] = 1
	v[63] = 0.5
	s := embeddingLiteral(v)
	if s[0] != '[' || s[len(s)-1] != ']' {
		t.Fatalf("literal must be bracketed: %s", s[:10])
	}
	if want := "[1.0000,"; s[:len(want)] != want {
		t.Errorf("literal prefix = %s, want %s", s[:len(want)], want)
	}
}
