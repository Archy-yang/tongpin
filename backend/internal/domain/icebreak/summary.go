package icebreak

import (
	"encoding/json"
	"fmt"
	"strings"
)

// summarySystemPrompt 摘要生成提示。
const summarySystemPrompt = `你是破冰分析器。根据两位用户的风格画像与两个 AI 代理的试聊记录，
输出 JSON 摘要，帮助两位真人开始对话。

规则：
- common_points：从画像与试聊中提炼 3 个真实共同点，口语化中文，不要编造
- topics：3 个双方都可能有话说的具体话题（要具体，如"最近在听的民谣"而不是"音乐"）
- cautions：2 个雷点/边界提醒（依据双方画像中的边界感、冲突风格、隐私敏感度，温和措辞）
- opener：1 个开场建议，给发起方用，一句话，自然不尴尬
- 只输出 JSON，不要解释文字

格式：
{"common_points": ["","",""], "topics": ["","",""], "cautions": ["",""], "opener": ""}`

// ParseSummary 解析并校验摘要 JSON。
func ParseSummary(raw string) (Summary, error) {
	var s Summary
	if err := json.Unmarshal([]byte(stripFences(raw)), &s); err != nil {
		return Summary{}, fmt.Errorf("parse summary: %w", err)
	}
	if len(s.CommonPoints) == 0 || len(s.Topics) == 0 || s.Opener == "" {
		return Summary{}, fmt.Errorf("summary missing required fields")
	}
	// 截断到约定数量，防 LLM 超发
	s.CommonPoints = clampList(s.CommonPoints, 3)
	s.Topics = clampList(s.Topics, 3)
	s.Cautions = clampList(s.Cautions, 2)
	return s, nil
}

func clampList(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	return strings.TrimSuffix(strings.TrimSpace(s), "```")
}

// agentSystemPrompt 单个 AI 代理的人格：以用户画像为性格进行试聊。
func agentSystemPrompt(profileJSON string, side string) string {
	return fmt.Sprintf(`你是一位用户的 AI 代理，正在与另一位 AI 代理试聊，帮各自的主人探探是否聊得来。
你的说话方式要贴合主人的风格画像：
%s

规则：
- 以第一人称聊天，但不要编造画像之外的具体个人事实（姓名、住址、工作单位等一律不编）
- 说话像真人微信聊天，1-2 句，自然
- 这只是试聊，不用刻意讨好，按画像的风格来
- 只输出你说的话本身`, profileJSON)
}

// buildAgentTurn 组装代理的续聊请求。
func buildAgentTurn(mySide, otherSide string, transcript []Turn) string {
	if len(transcript) == 0 {
		return "试聊开始。由你先开口，打个自然的招呼。"
	}
	var b strings.Builder
	b.WriteString("目前试聊记录：\n")
	for _, t := range transcript {
		who := "对方"
		if t.Side == mySide {
			who = "你"
		}
		fmt.Fprintf(&b, "[%s] %s\n", who, t.Text)
	}
	b.WriteString("\n轮到你了，接着说一两句。")
	return b.String()
}

// buildSummaryUser 组装摘要请求：双方画像 + 完整试聊记录。
func buildSummaryUser(profileA, profileB []byte, transcript []Turn) string {
	var b strings.Builder
	fmt.Fprintf(&b, "用户A（发起方）画像：\n%s\n\n用户B 画像：\n%s\n\nAI 代理试聊记录：\n", profileA, profileB)
	for _, t := range transcript {
		who := "A 的代理"
		if t.Side == "b" {
			who = "B 的代理"
		}
		fmt.Fprintf(&b, "[%s] %s\n", who, t.Text)
	}
	return b.String()
}
