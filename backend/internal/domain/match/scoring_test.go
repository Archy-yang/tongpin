package match

import (
	"math"
	"testing"
)

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCosine(t *testing.T) {
	identical := cosine([]float64{1, 0, 1}, []float64{1, 0, 1})
	if !approxEq(identical, 1) {
		t.Errorf("identical vectors cosine = %v, want 1", identical)
	}
	orthogonal := cosine([]float64{1, 0}, []float64{0, 1})
	if !approxEq(orthogonal, 0) {
		t.Errorf("orthogonal vectors cosine = %v, want 0", orthogonal)
	}
	if cosine(nil, nil) != 0 {
		t.Error("zero vectors should yield 0")
	}
}

func TestIntentScore(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"serious", "serious", 1},
		{"serious", "serious_but_slow", 0.7},
		{"serious_but_slow", "serious", 0.7}, // 对称
		{"friends", "casual", 0.5},
		{"friends", "serious", 0.4},
		{"casual", "serious", 0},
		{"", "serious", 0},
	}
	for _, c := range cases {
		if got := intentScore(c.a, c.b); got != c.want {
			t.Errorf("intentScore(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestHoursOverlap(t *testing.T) {
	full := hoursOverlap([]string{"a", "b"}, []string{"a", "b"})
	if full != 1 {
		t.Errorf("identical sets = %v, want 1", full)
	}
	half := hoursOverlap([]string{"a", "b"}, []string{"a", "c"})
	// Jaccard：inter=1, union=3 → 1/3
	if !approxEq(half, 1.0/3.0) {
		t.Errorf("partial overlap = %v, want 1/3", half)
	}
	if hoursOverlap(nil, []string{"a"}) != 0 {
		t.Error("empty set should yield 0")
	}
}

func TestComplementScore(t *testing.T) {
	if got := complementScore(0.5, 0.9); got != 1 {
		t.Errorf("diff 0.4 = %v, want 1 (适中差异满分)", got)
	}
	if got := complementScore(0.5, 0.5); got != 0 {
		t.Errorf("no diff should score 0 (无差异无新鲜感), got %v", got)
	}
	if got := complementScore(0, 1); got != 0 {
		t.Errorf("extreme diff = %v, want 0", got)
	}
}

func TestParseAgeRange(t *testing.T) {
	cases := []struct{ in string; lo, hi int }{
		{"25-30", 25, 30},
		{"40+", 40, 120},
		{"", 0, 120},
		{"garbage", 0, 120},
	}
	for _, c := range cases {
		lo, hi := parseAgeRange(c.in)
		if lo != c.lo || hi != c.hi {
			t.Errorf("parseAgeRange(%q) = (%d,%d), want (%d,%d)", c.in, lo, hi, c.lo, c.hi)
		}
	}
}

func TestHardFilter(t *testing.T) {
	base := func(c *Candidate) { *c = Candidate{AcceptRealContact: true, Gender: "female", AgeRange: "25-30"} }
	me := Candidate{}
	base(&me)
	me.Gender = "male"
	me.GenderPref = "female"
	me.AgeRange = "28-33"

	other := Candidate{}
	base(&other)
	other.GenderPref = "male"

	if !hardFilter(me, other) {
		t.Error("compatible pair should pass")
	}

	// 性别偏好不匹配
	other.GenderPref = "female"
	if hardFilter(me, other) {
		t.Error("mismatched gender pref should fail")
	}
	other.GenderPref = "male"

	// 年龄偏好不重叠
	other.AgePrefMin = intPtr(40)
	if hardFilter(me, other) {
		t.Error("non-overlapping age pref should fail")
	}
	other.AgePrefMin = nil

	// 拒绝真人接触
	other.AcceptRealContact = false
	if hardFilter(me, other) {
		t.Error("rejecting real contact should fail")
	}
	other.AcceptRealContact = true

	// 年龄区间本身不重叠：我 18-22，对方偏好 40+
	me.AgeRange = "18-22"
	other.AgePrefMin, other.AgePrefMax = intPtr(40), intPtr(50)
	if hardFilter(me, other) {
		t.Error("my age below their pref should fail")
	}
}

func TestGenderOK(t *testing.T) {
	if !genderOK("female", "female") {
		t.Error("exact match should pass")
	}
	if genderOK("female", "male") {
		t.Error("mismatch should fail")
	}
	if !genderOK("other", "male") {
		t.Error("pref other = no filter")
	}
	if !genderOK("female", "") {
		t.Error("gender unset = no filter")
	}
}

func TestBuildHighlights(t *testing.T) {
	me := MatchProfile{
		Intent: "serious", Pace: "slow",
		Topics:      []string{"music", "philosophy", "food"},
		ActiveHours: []string{"late_night"},
	}
	other := MatchProfile{
		Intent: "serious", Pace: "slow",
		Topics:      []string{"philosophy", "movies"},
		ActiveHours: []string{"late_night", "evening"},
	}
	hs := buildHighlights(me, other, "北京", "北京")
	if len(hs) != 3 {
		t.Fatalf("highlights = %v, want 3 (5 项候选截取前 3)", hs)
	}
	// 顺序：共同话题 → 同意图 → 同节奏（时段/城市被截断）
	joined := hs[0] + hs[1] + hs[2]
	for _, want := range []string{"哲学", "认真的关系", "慢节奏"} {
		if !contains(joined, want) {
			t.Errorf("highlights missing %q: %v", want, hs)
		}
	}
}

func TestParseEmbedding(t *testing.T) {
	v := parseEmbedding("[0.1, 0.2,0.3]")
	if len(v) != 3 || v[0] != 0.1 || v[2] != 0.3 {
		t.Errorf("parseEmbedding = %v", v)
	}
	if got := parseEmbedding("[]"); len(got) != 0 {
		t.Errorf("empty vector should be empty, got %v", got)
	}
}

func intPtr(v int) *int { return &v }

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
