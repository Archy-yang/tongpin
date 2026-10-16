package match

import (
	"fmt"
	"math"
	"strings"
)

// 匹配总分权重（见 docs/product/03-style-profile.md / 技术架构文档）
const (
	wCosine      = 0.45
	wIntent      = 0.20
	wHours       = 0.15
	wGeo         = 0.10
	wComplement  = 0.10
)

// ScoreBreakdown 各分项得分，存入 matches.score_detail 供调参复盘。
// PrefCos/PrefWeight 为偏好协同项（样本足够时才非零）：
// total = (1-prefWeight) × 五项基础分 + prefWeight × PrefCos。
type ScoreBreakdown struct {
	Cosine     float64  `json:"cosine"`
	Intent     float64  `json:"intent"`
	Hours      float64  `json:"hours"`
	Geo        float64  `json:"geo"`
	Complement float64  `json:"complement"`
	PrefCos    float64  `json:"pref_cos,omitempty"`
	PrefWeight float64  `json:"pref_weight,omitempty"`
	Highlights []string `json:"highlights"`
}

func (s ScoreBreakdown) Total() float64 {
	return wCosine*s.Cosine + wIntent*s.Intent + wHours*s.Hours +
		wGeo*s.Geo + wComplement*s.Complement
}

// cosine 余弦相似度（两向量等长）。
func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// intentScore 关系意图兼容度（对称矩阵）。
// serious 与 serious_but_slow 高兼容；casual 与 serious 不兼容。
func intentScore(a, b string) float64 {
	if a == b {
		return 1
	}
	pair := a + "|" + b
	if pair == "serious|serious_but_slow" || pair == "serious_but_slow|serious" {
		return 0.7
	}
	if pair == "friends|serious" || pair == "serious|friends" ||
		pair == "friends|serious_but_slow" || pair == "serious_but_slow|friends" {
		return 0.4
	}
	if pair == "casual|friends" || pair == "friends|casual" {
		return 0.5
	}
	return 0 // casual|serious 及其余组合
}

// hoursOverlap 活跃时段 Jaccard 重叠。
func hoursOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, h := range a {
		setA[h] = true
	}
	inter := 0
	for _, h := range b {
		if setA[h] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

// complementScore 互补/新鲜感：主动性差异适中（0.2-0.6）加分，过大过小衰减。
func complementScore(a, b float64) float64 {
	diff := math.Abs(a - b)
	return math.Max(0, 1-math.Abs(diff-0.4)*2.5)
}

// parseAgeRange "25-30" → (25,30)；"40+" → (40,120)；空 → (0,120) 不设限。
func parseAgeRange(s string) (int, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 120
	}
	if strings.HasSuffix(s, "+") {
		var lo int
		if _, err := fmt.Sscanf(s, "%d+", &lo); err == nil {
			return lo, 120
		}
		return 0, 120
	}
	var lo, hi int
	if _, err := fmt.Sscanf(s, "%d-%d", &lo, &hi); err == nil {
		return lo, hi
	}
	return 0, 120
}

// hardFilter 硬条件：性别偏好（双向）、年龄段重叠、双方都接受真人接触。
// gender / genderPref 为 "male"|"female"|"other"|"unset"|""，other/unset 视为不过滤。
func hardFilter(me, other Candidate) bool {
	if !me.AcceptRealContact || !other.AcceptRealContact {
		return false
	}
	if !genderOK(me.GenderPref, other.Gender) || !genderOK(other.GenderPref, me.Gender) {
		return false
	}
	loA, hiA := parseAgeRange(me.AgeRange)
	loB, hiB := parseAgeRange(other.AgeRange)
	if me.AgePrefMin != nil && hiB < *me.AgePrefMin {
		return false
	}
	if me.AgePrefMax != nil && loB > *me.AgePrefMax {
		return false
	}
	// 对方的偏好范围与我的实际年龄区间需有交集
	if other.AgePrefMin != nil && hiA < *other.AgePrefMin {
		return false
	}
	if other.AgePrefMax != nil && loA > *other.AgePrefMax {
		return false
	}
	return true
}

func genderOK(pref, gender string) bool {
	if pref == "" || pref == "other" || pref == "unset" ||
		gender == "other" || gender == "unset" || gender == "" {
		return true
	}
	return pref == gender
}

// buildHighlights 生成契合点文案（代码规则生成，P1 才引入 AI 暖场摘要）。
func buildHighlights(me, other MatchProfile, cityA, cityB string) []string {
	var out []string

	topicNames := map[string]string{
		"music": "音乐", "movies": "电影", "books": "读书", "games": "游戏",
		"sports": "运动", "travel": "旅行", "food": "美食", "philosophy": "哲学思辨",
		"technology": "科技", "art": "艺术", "late_night_thoughts": "深夜漫谈",
		"emotions": "情感话题", "career": "职业发展", "life": "日常生活",
	}
	common := intersect(me.Topics, other.Topics)
	for i, t := range common {
		if i >= 2 {
			break
		}
		if name, ok := topicNames[t]; ok {
			out = append(out, "你们都对"+name+"有共同兴趣")
		}
	}

	if me.Intent != "" && me.Intent == other.Intent {
		intents := map[string]string{
			"serious": "你们都想要一段认真的关系", "serious_but_slow": "你们都想慢慢来、认真了解对方",
			"friends": "你们都先从朋友做起", "casual": "你们都想轻松地聊聊",
		}
		out = append(out, intents[me.Intent])
	}
	if me.Pace != "" && me.Pace == other.Pace {
		paces := map[string]string{"slow": "你们都偏好慢节奏的对话", "medium": "你们的聊天节奏相近", "fast": "你们都是快节奏的聊天者"}
		out = append(out, paces[me.Pace])
	}
	if shared := intersect(me.ActiveHours, other.ActiveHours); len(shared) > 0 {
		buckets := map[string]string{"late_night": "你们都在深夜活跃", "evening": "你们都常在晚间在线",
			"morning": "你们都习惯早起交流", "afternoon": "你们常在午后在线"}
		out = append(out, buckets[shared[0]])
	}
	if cityA != "" && cityA == cityB {
		out = append(out, "你们在同一个城市")
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func intersect(a, b []string) []string {
	setB := make(map[string]bool, len(b))
	for _, v := range b {
		setB[v] = true
	}
	var out []string
	for _, v := range a {
		if setB[v] {
			out = append(out, v)
		}
	}
	return out
}
