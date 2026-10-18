package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Service struct {
	repo      *Repository
	providers map[string]PaymentProvider // 渠道适配器，装配期注册
	gate      AccountGate                // 可为 nil（关闭购买前绑邮箱门槛）
}

// AccountGate 购买门槛：支付必须挂在可恢复的账号上。
type AccountGate interface {
	HasEmail(ctx context.Context, userID int64) (bool, error)
}

var ErrEmailRequired = errors.New("email required before checkout")

// SetAccountGate 装配期注入购买门槛。
func (s *Service) SetAccountGate(g AccountGate) { s.gate = g }

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, providers: make(map[string]PaymentProvider)}
}

// Register 装配期注册支付渠道适配器。
func (s *Service) Register(p PaymentProvider) {
	s.providers[p.Name()] = p
}

func (s *Service) provider(name string) (PaymentProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, ErrProviderUnavailable
	}
	return p, nil
}

// StartCheckout 发起一次会员购买：建收银台会话，返回跳转 URL。
func (s *Service) StartCheckout(ctx context.Context, userID int64, plan string) (string, error) {
	// 付款必须挂在可恢复的账号上：先绑定邮箱，再谈钱
	if s.gate != nil {
		bound, err := s.gate.HasEmail(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("account gate: %w", err)
		}
		if !bound {
			return "", ErrEmailRequired
		}
	}
	if plan == "" {
		plan = PlanMemberMonthly
	}
	p, err := s.provider("stripe") // MVP 只有 Stripe 在线渠道；网页端已带 Apple Pay/Google Pay
	if err != nil {
		return "", err
	}
	return p.CreateCheckout(ctx, userID, plan)
}

// ParseWebhook 委托渠道适配器验签并归一化事件。
func (s *Service) ParseWebhook(providerName string, header http.Header, body []byte) (Event, error) {
	p, err := s.provider(providerName)
	if err != nil {
		return Event{}, err
	}
	ev, err := p.ParseWebhook(header, body)
	if err != nil {
		return Event{}, err
	}
	if ev.Provider == "" {
		ev.Provider = providerName
	}
	return ev, nil
}

// HandleEvent 推进订阅状态机。幂等：重复事件（三方会重投）直接跳过。
// 返回错误 = 未处理成功，三方会按自己的策略重试。
func (s *Service) HandleEvent(ctx context.Context, ev Event, raw []byte) error {
	claimed, err := s.repo.ClaimEvent(ctx, ev.ID, ev.Provider, raw)
	if err != nil {
		return fmt.Errorf("claim event: %w", err)
	}
	if !claimed {
		return nil // 已处理过的重复投递
	}

	sub := Subscription{
		UserID:           ev.UserID,
		Provider:         ev.Provider,
		ExternalID:       ev.ExternalID,
		Plan:             PlanMemberMonthly,
		CurrentPeriodEnd: ev.PeriodEnd,
	}
	switch ev.Type {
	case EventSubscriptionUpserted:
		sub.Status = ev.Status
		if sub.UserID > 0 {
			err = s.repo.UpsertSubscription(ctx, sub)
		} else {
			err = s.repo.UpdateStatusByExternalID(ctx, sub.Provider, sub.ExternalID, sub.Status, sub.CurrentPeriodEnd)
		}
	case EventSubscriptionCanceled:
		sub.Status = StatusCanceled
		if sub.UserID > 0 {
			err = s.repo.UpsertSubscription(ctx, sub)
		} else {
			err = s.repo.UpdateStatusByExternalID(ctx, sub.Provider, sub.ExternalID, StatusCanceled, sub.CurrentPeriodEnd)
		}
	default:
		return fmt.Errorf("unknown event type %q", ev.Type)
	}
	if err != nil {
		return fmt.Errorf("apply event: %w", err)
	}
	log.Printf("[billing] %s %s: sub %s (user %d) -> %s", ev.Provider, ev.Type, ev.ExternalID, ev.UserID, sub.Status)
	return nil
}

// Entitlement 会员权益判断（chat 域经消费方接口消费，billing 不感知 chat）。
func (s *Service) Entitlement(ctx context.Context, userID int64) (bool, error) {
	return s.repo.HasActive(ctx, userID)
}

// Current 用户当前订阅的展示视图（没有订阅时返回零值 DTO）。
func (s *Service) Current(ctx context.Context, userID int64) (SubscriptionDTO, error) {
	sub, err := s.repo.CurrentOf(ctx, userID)
	if err != nil || sub == nil {
		return SubscriptionDTO{}, err
	}
	return SubscriptionDTO{
		Active:           sub.Status == StatusActive && sub.CurrentPeriodEnd.After(time.Now()),
		Plan:             sub.Plan,
		Status:           sub.Status,
		CurrentPeriodEnd: sub.CurrentPeriodEnd.Format(time.RFC3339),
	}, nil
}
