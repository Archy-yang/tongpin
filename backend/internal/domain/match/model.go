// Package match 匹配域：匹配池 opt-in、硬条件过滤、风格向量召回与打分、契合提示。
package match

import (
	"encoding/json"
	"time"
)

// ---- 匹配池 opt-in ----

var validIntents = map[string]bool{
	"casual": true, "friends": true, "serious": true, "serious_but_slow": true,
}

type OptIn struct {
	UserID            int64
	Intent            string
	AgePrefMin        *int
	AgePrefMax        *int
	AcceptRealContact bool
	Status            string
}

type OptInRequest struct {
	Intent            string `json:"relationship_intent" binding:"required"`
	AgePrefMin        *int   `json:"age_pref_min" binding:"omitempty,gte=18,lte=100"`
	AgePrefMax        *int   `json:"age_pref_max" binding:"omitempty,gte=18,lte=120"`
	AcceptRealContact *bool  `json:"accept_real_contact"`
}

type OptInView struct {
	Intent            string `json:"relationship_intent"`
	AgePrefMin        *int   `json:"age_pref_min,omitempty"`
	AgePrefMax        *int   `json:"age_pref_max,omitempty"`
	AcceptRealContact bool   `json:"accept_real_contact"`
	Status            string `json:"status"`
}

// ---- 契合提示 ----

type Suggestion struct {
	MatchID   int64    `json:"match_id"`
	Score     float64  `json:"score"`
	Highlights []string `json:"highlights"`
	CreatedAt time.Time `json:"created_at"`
}

type SuggestionsView struct {
	PoolStatus  string       `json:"pool_status"` // active|warming_up
	ActiveUsers int          `json:"active_users"`
	Suggestions []Suggestion `json:"suggestions"`
}

// ---- 打分输入：从画像 JSON 中解析本域关心的字段（本地定义，不 import style 的类型）----

type MatchProfile struct {
	Intent      string   `json:"relationship_intent"`
	ActiveHours []string `json:"active_hours"`
	Topics      []string `json:"topic_preferences"`
	Pace        string   `json:"pace"`
	Tone        string   `json:"tone"`
	Initiative  float64  `json:"initiative"`
}

func parseMatchProfile(raw []byte) MatchProfile {
	var p struct {
		CommunicationStyle struct {
			Pace       string  `json:"pace"`
			Initiative float64 `json:"initiative"`
		} `json:"communication_style"`
		EmotionalStyle struct {
			Tone string `json:"tone"`
		} `json:"emotional_style"`
		TopicPreferences   []string `json:"topic_preferences"`
		RelationshipIntent string   `json:"relationship_intent"`
		ActiveHours        []string `json:"active_hours"`
	}
	_ = json.Unmarshal(raw, &p)
	return MatchProfile{
		Intent:      p.RelationshipIntent,
		ActiveHours: p.ActiveHours,
		Topics:      p.TopicPreferences,
		Pace:        p.CommunicationStyle.Pace,
		Tone:        p.EmotionalStyle.Tone,
		Initiative:  p.CommunicationStyle.Initiative,
	}
}
