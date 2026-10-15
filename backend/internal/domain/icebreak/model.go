// Package icebreak AI 暖场域：双方同意后，两个 AI 代理以其用户画像为性格
// 破冰对话 3 轮，产出结构化摘要供真人参考。红线：AI 只互聊与总结，
// 绝不代替用户向对方发消息。
package icebreak

// SessionStatus 状态机：pending → generating → ready | declined | failed
const (
	StatusPending    = "pending"
	StatusGenerating = "generating"
	StatusReady      = "ready"
	StatusDeclined   = "declined"
	StatusFailed     = "failed"
)

type Session struct {
	ID        int64   `json:"id"`
	MatchID   int64   `json:"match_id"`
	FromUser  int64   `json:"from_user"`
	ToUser    int64   `json:"to_user"`
	Status    string  `json:"status"`
	Transcript []Turn `json:"transcript,omitempty"`
	Summary   *Summary `json:"summary,omitempty"`
}

// Turn 双 AI 对话中的一句。side = "a"（发起方代理）| "b"（同意方代理）。
type Turn struct {
	Side string `json:"side"`
	Text string `json:"text"`
}

// Summary 破冰摘要（与 docs/product/01 的 AI 暖场输出定义一致）。
type Summary struct {
	CommonPoints []string `json:"common_points"` // 3 个共同点
	Topics       []string `json:"topics"`        // 3 个可聊话题
	Cautions     []string `json:"cautions"`      // 2 个雷点/边界提醒
	Opener       string   `json:"opener"`        // 1 个开场建议
}
