package billing

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// 订阅事件的两种归一化形态：三方状态变更统一进 Upserted，终结进 Canceled。
// 各渠道的原始事件类型由适配器翻译（如 stripe 的 customer.subscription.updated）。
type EventType string

const (
	EventSubscriptionUpserted EventType = "subscription_upserted"
	EventSubscriptionCanceled EventType = "subscription_canceled"
)

// Event 归一化后的支付事件（webhook 的域内表示）。
type Event struct {
	ID         string    // 三方事件唯一 ID，幂等键
	Type       EventType
	Provider   string
	UserID     int64     // 可能为 0（事件体没带用户时，按 ExternalID 回查）
	ExternalID string    // 三方订阅标识
	Status     string    // Upserted 时有效：active | past_due | incomplete
	PeriodEnd  time.Time // 当前周期截止；零值 = 未知
}

// PaymentProvider 支付渠道适配接口（消费方定义，装配层注入实现）。
// 三家渠道实现同一接口：新增渠道 = 新适配器 + 一行装配，域内零改动。
type PaymentProvider interface {
	Name() string
	// CreateCheckout 创建一次收银台会话，返回跳转 URL。
	CreateCheckout(ctx context.Context, userID int64, plan string) (redirectURL string, err error)
	// ParseWebhook 验签并把原始请求归一化为 Event。
	// 与本域无关的渠道事件返回 ErrEventIgnored（域直接静默确认）。
	ParseWebhook(header http.Header, body []byte) (Event, error)
}

var (
	// ErrProviderUnavailable 支付渠道未配置（缺密钥等），对外表现为"支付暂未开放"。
	ErrProviderUnavailable = errors.New("payment provider unavailable")
	// ErrEventIgnored 事件与本域无关（适配器已验签，只是不做业务处理）。
	ErrEventIgnored = errors.New("event ignored")
	// ErrWebhookInvalid 验签失败或报文损坏——必须拒绝并让三方重试/告警。
	ErrWebhookInvalid = errors.New("webhook invalid")
)
