// Package stripe Stripe 支付渠道适配器：实现 billing.PaymentProvider。
// 不引入 stripe-go SDK——Stripe 的 REST API 与 webhook 验签都很薄，
// 手搓保持依赖干净（与 mailer/jwt/llm 同一哲学）。
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Archy-yang/tongpin/backend/internal/domain/billing"
)

const (
	apiBase      = "https://api.stripe.com/v1"
	sigTolerance = 5 * time.Minute // Stripe 官方推荐的时间戳容差
)

type Config struct {
	SecretKey     string // sk_test_... / sk_live_...
	WebhookSecret string // whsec_...
	PriceID       string // price_...（会员月度价格）
	SuccessURL    string // 支付成功跳转（网页）
	CancelURL     string // 取消跳转
	Timeout       time.Duration
}

type Provider struct {
	cfg Config
	hc  *http.Client
}

func New(cfg Config) *Provider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Provider{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

func (p *Provider) Name() string { return "stripe" }

// CreateCheckout 建一次订阅收银台会话（Stripe Checkout 托管页）。
// user_id 同时写进 client_reference_id 与 subscription_data.metadata——
// 后续所有 subscription.* 生命周期事件都靠 metadata 带回用户。
func (p *Provider) CreateCheckout(ctx context.Context, userID int64, plan string) (string, error) {
	if p.cfg.SecretKey == "" || p.cfg.PriceID == "" {
		return "", billing.ErrProviderUnavailable
	}
	uid := strconv.FormatInt(userID, 10)
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", p.cfg.PriceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("client_reference_id", uid)
	form.Set("subscription_data[metadata][user_id]", uid)
	form.Set("success_url", p.cfg.SuccessURL)
	form.Set("cancel_url", p.cfg.CancelURL)

	var out struct {
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := p.call(ctx, "POST", "/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("stripe: %s", out.Error.Message)
	}
	return out.URL, nil
}

// ParseWebhook 验签 → 归一化。只处理 subscription 生命周期事件，
// 其余（invoice.*、checkout.session.completed 等）静默忽略——
// 订阅的开通与变更由 customer.subscription.created/updated 完整携带。
func (p *Provider) ParseWebhook(header http.Header, body []byte) (billing.Event, error) {
	if p.cfg.WebhookSecret == "" {
		return billing.Event{}, billing.ErrProviderUnavailable
	}
	if err := verifySignature(header.Get("Stripe-Signature"), body, p.cfg.WebhookSecret); err != nil {
		return billing.Event{}, errors.Join(billing.ErrWebhookInvalid, err)
	}

	var env struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID               string            `json:"id"`
				Status           string            `json:"status"`
				ClientReference  string            `json:"client_reference_id"`
				CurrentPeriodEnd int64             `json:"current_period_end"`
				Metadata         map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return billing.Event{}, errors.Join(billing.ErrWebhookInvalid, err)
	}

	userID := parseUserID(env.Data.Object.Metadata["user_id"], env.Data.Object.ClientReference)
	ev := billing.Event{
		ID:         env.ID,
		Provider:   p.Name(),
		UserID:     userID,
		ExternalID: env.Data.Object.ID,
		PeriodEnd:  unixToTime(env.Data.Object.CurrentPeriodEnd),
	}

	switch env.Type {
	case "customer.subscription.created", "customer.subscription.updated":
		switch env.Data.Object.Status {
		case "active", "trialing":
			ev.Type, ev.Status = billing.EventSubscriptionUpserted, billing.StatusActive
		case "past_due", "unpaid":
			ev.Type, ev.Status = billing.EventSubscriptionUpserted, billing.StatusPastDue
		case "incomplete":
			ev.Type, ev.Status = billing.EventSubscriptionUpserted, billing.StatusIncomplete
		case "incomplete_expired", "canceled":
			ev.Type = billing.EventSubscriptionCanceled
		default:
			return billing.Event{}, billing.ErrEventIgnored
		}
	case "customer.subscription.deleted":
		ev.Type = billing.EventSubscriptionCanceled
	default:
		return billing.Event{}, billing.ErrEventIgnored
	}
	return ev, nil
}

// call 执行一次 Stripe REST 调用（表单编码，API key 鉴权）。
func (p *Provider) call(ctx context.Context, method, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.hc.Do(req)
	if err != nil {
		return fmt.Errorf("stripe request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// verifySignature 校验 Stripe-Signature: t=<unix>,v1=<hex>。
// 期望签名 = HMAC-SHA256(webhook_secret, t + "." + body)，常量时间比较，时间戳限容差内。
func verifySignature(sigHeader string, body []byte, secret string) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(sigHeader, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return errors.New("missing signature elements")
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("bad timestamp")
	}
	if diff := time.Since(time.Unix(unix, 0)); diff > sigTolerance || diff < -sigTolerance {
		return errors.New("timestamp outside tolerance")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	expect := mac.Sum(nil)

	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(expect, got) == 1 {
			return nil
		}
	}
	return errors.New("signature mismatch")
}

func parseUserID(meta, ref string) int64 {
	for _, v := range []string{meta, ref} {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func unixToTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
