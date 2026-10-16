// Package billing 订阅与支付域：状态机、幂等与权益查询。
// 支付渠道（Stripe/Apple/Google）只通过 PaymentProvider 接口进入本域，
// 适配器实现在 internal/payments/，由装配层注册——本域不知道任何具体渠道。
package billing

import "time"

// Plan MVP 只有一个付费计划：会员月度。
const PlanMemberMonthly = "member_monthly"

// 订阅状态机：incomplete → active →（扣款失败）past_due →（最终）canceled。
const (
	StatusIncomplete = "incomplete"
	StatusActive     = "active"
	StatusPastDue    = "past_due"
	StatusCanceled   = "canceled"
)

type Subscription struct {
	ID               int64
	UserID           int64
	Provider         string
	ExternalID       string
	Status           string
	Plan             string
	CurrentPeriodEnd time.Time
}

// SubscriptionDTO 对外展示视图（前端会员卡）。
type SubscriptionDTO struct {
	Active           bool   `json:"active"`
	Plan             string `json:"plan,omitempty"`
	Status           string `json:"status,omitempty"`
	CurrentPeriodEnd string `json:"current_period_end,omitempty"` // RFC3339
}
