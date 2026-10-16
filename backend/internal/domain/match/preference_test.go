package match

import (
	"math"
	"testing"
)

func vec3(a, b, c float64) []float64 { return []float64{a, b, c} }

func TestPreferenceVector(t *testing.T) {
	// 两个 like（同为 [1,0,0] 方向）+ 一个 mismatch（[0,1,0]）
	entries := []FeedbackEntry{
		{Direction: "like", Embedding: vec3(1, 0, 0)},
		{Direction: "like", Embedding: vec3(1, 0, 0)},
		{Direction: "mismatch", Embedding: vec3(0, 1, 0)},
	}
	v, likes, mism, ready := PreferenceVector(entries)
	if !ready || likes != 2 || mism != 1 {
		t.Fatalf("ready=%v likes=%d mism=%d", ready, likes, mism)
	}
	// 加权和 = [2,-0.5,0]，归一化后第一个分量应显著为正、第二个为负
	if v[0] <= 0 || v[1] >= 0 {
		t.Errorf("preference direction wrong: %v", v)
	}
	// 单位向量
	norm := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if math.Abs(norm-1) > 1e-9 {
		t.Errorf("not normalized: %v", norm)
	}
}

func TestPreferenceVectorAllMismatch(t *testing.T) {
	entries := []FeedbackEntry{
		{Direction: "mismatch", Embedding: vec3(1, 0, 0)},
		{Direction: "mismatch", Embedding: vec3(0, 1, 0)},
	}
	_, _, _, ready := PreferenceVector(entries)
	if ready {
		t.Error("all-mismatch must not define a preference")
	}
}

func TestPreferenceVectorEmpty(t *testing.T) {
	if _, _, _, ready := PreferenceVector(nil); ready {
		t.Error("no entries must not be ready")
	}
}

func TestPrefWeightRamp(t *testing.T) {
	cases := []struct{ likes, mism int; want float64 }{
		{1, 0, 0},     // 样本不足
		{2, 0, 0},     // 样本不足
		{3, 0, 0.09},  // 3/5 爬坡：0.15*0.6
		{4, 1, 0.15},  // 5 条爬满（4+1=5）
		{10, 0, 0.15}, // 封顶
	}
	for _, c := range cases {
		if got := prefWeightFor(c.likes, c.mism); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("prefWeightFor(%d,%d) = %v, want %v", c.likes, c.mism, got, c.want)
		}
	}
}

func TestEmbeddingLiteralMatch(t *testing.T) {
	s := embeddingLiteral([]float64{1, 0.5})
	if s != "[1.0000,0.5000]" {
		t.Errorf("literal = %q", s)
	}
}
