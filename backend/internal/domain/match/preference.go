package match

import (
	"fmt"
	"math"
	"strings"
)

// FeedbackEntry 一条聊后反馈及其对象（匹配对手）的画像嵌入。
type FeedbackEntry struct {
	Direction string // like | mismatch
	Embedding []float64
}

const (
	prefMinSamples   = 3    // 样本少于该值不启用偏好信号
	prefMaxWeight    = 0.15 // 偏好项权重上限
	prefRampSamples  = 5.0  // 样本达到该数量时权重爬满
	mismatchWeight   = -0.5
)

// PreferenceVector 计算用户偏好向量：加权平均后归一化。
// 只有 like（净权重为正）才定义"喜欢什么"；全是不喜欢则视为无偏好。
// 返回 (向量, like 数, mismatch 数, 是否可用)。
func PreferenceVector(entries []FeedbackEntry) ([]float64, int, int, bool) {
	var acc []float64
	wSum := 0.0
	likes, mismatches := 0, 0

	for _, e := range entries {
		if len(e.Embedding) == 0 {
			continue
		}
		w := 1.0
		if e.Direction == "mismatch" {
			w = mismatchWeight
			mismatches++
		} else {
			likes++
		}
		if acc == nil {
			acc = make([]float64, len(e.Embedding))
		}
		for i, v := range e.Embedding {
			acc[i] += w * v
		}
		wSum += w
	}
	if acc == nil || wSum <= 0 {
		return nil, likes, mismatches, false
	}

	norm := 0.0
	for _, v := range acc {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return nil, likes, mismatches, false
	}
	for i := range acc {
		acc[i] /= norm
	}
	return acc, likes, mismatches, true
}

// prefWeightFor 样本量决定偏好项权重：3 条起生效，5 条爬满 0.15。
func prefWeightFor(likes, mismatches int) float64 {
	n := likes + mismatches
	if n < prefMinSamples {
		return 0
	}
	return prefMaxWeight * math.Min(1, float64(n)/prefRampSamples)
}

// embeddingLiteral 向量 → pgvector 字面量（与 style 域格式一致；跨域不 import，小函数就地实现）。
func embeddingLiteral(v []float64) string {
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
