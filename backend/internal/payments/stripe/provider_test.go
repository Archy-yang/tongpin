package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Archy-yang/tongpin/backend/internal/domain/billing"
)

const testSecret = "whsec_test_secret"

// sign 按 Stripe 的规则生成 Stripe-Signature 头（t=...,v1=...）。
func sign(t *testing.T, body []byte, at time.Time) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSecret))
	fmt.Fprintf(mac, "%d", at.Unix())
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", at.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func headers(sig string) http.Header {
	h := http.Header{}
	h.Set("Stripe-Signature", sig)
	return h
}

func subscriptionUpdatedJSON(id, status string, userID int64, periodEnd int64) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "evt_%s", "type": "customer.subscription.updated",
		"data": {"object": {
			"id": "%s", "status": "%s", "current_period_end": %d,
			"metadata": {"user_id": "%d"}
		}}
	}`, id, id, status, periodEnd, userID))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"type":"customer.subscription.updated"}`)
	now := time.Now()

	if err := verifySignature(sign(t, body, now), body, testSecret); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	// 篡改报文：签名对不上
	if err := verifySignature(sign(t, body, now), []byte(`{"type":"tampered"}`), testSecret); err == nil {
		t.Fatal("tampered body accepted")
	}
	// 错误密钥
	if err := verifySignature(sign(t, body, now), body, "whsec_wrong"); err == nil {
		t.Fatal("wrong secret accepted")
	}
	// 时间戳超出容差（6 分钟前）
	if err := verifySignature(sign(t, body, now.Add(-6*time.Minute)), body, testSecret); err == nil {
		t.Fatal("stale timestamp accepted")
	}
	// 缺头
	if err := verifySignature("", body, testSecret); err == nil {
		t.Fatal("missing header accepted")
	}
}

func TestParseWebhookNormalizesSubscriptionEvents(t *testing.T) {
	p := New(Config{WebhookSecret: testSecret})
	body := subscriptionUpdatedJSON("sub_123", "active", 42, time.Now().Add(30*24*time.Hour).Unix())
	ev, err := p.ParseWebhook(headers(sign(t, body, time.Now())), body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Type != billing.EventSubscriptionUpserted || ev.Status != billing.StatusActive {
		t.Fatalf("type/status = %s/%s, want upserted/active", ev.Type, ev.Status)
	}
	if ev.UserID != 42 || ev.ExternalID != "sub_123" || ev.Provider != "stripe" {
		t.Fatalf("user/external/provider = %d/%s/%s", ev.UserID, ev.ExternalID, ev.Provider)
	}
	if ev.PeriodEnd.IsZero() {
		t.Fatal("period end lost")
	}
}

func TestParseWebhookStatusMapping(t *testing.T) {
	p := New(Config{WebhookSecret: testSecret})
	now := time.Now()
	cases := []struct {
		status     string
		eventType  string
		wantStatus string
		canceled   bool
	}{
		{"trialing", "customer.subscription.created", billing.StatusActive, false},
		{"past_due", "customer.subscription.updated", billing.StatusPastDue, false},
		{"incomplete", "customer.subscription.updated", billing.StatusIncomplete, false},
		{"incomplete_expired", "customer.subscription.updated", "", true},
	}
	for _, c := range cases {
		body := subscriptionUpdatedJSON("s_"+c.status, c.status, 7, 0)
		ev, err := p.ParseWebhook(headers(sign(t, body, now)), body)
		if err != nil {
			t.Fatalf("%s: parse: %v", c.status, err)
		}
		if c.canceled && ev.Type != billing.EventSubscriptionCanceled {
			t.Fatalf("%s: want canceled, got %s", c.status, ev.Type)
		}
		if !c.canceled && (ev.Type != billing.EventSubscriptionUpserted || ev.Status != c.wantStatus) {
			t.Fatalf("%s: got %s/%s", c.status, ev.Type, ev.Status)
		}
	}
}

func TestParseWebhookRejectedAndIgnored(t *testing.T) {
	p := New(Config{WebhookSecret: testSecret})
	body := subscriptionUpdatedJSON("u2", "sub_x", 1, 0)

	// 验签失败
	if _, err := p.ParseWebhook(headers("t=1,v1=deadbeef"), body); err == nil {
		t.Fatal("bad signature accepted")
	}
	// 无关事件：验签通过但静默忽略
	other := []byte(`{"id":"evt_i","type":"invoice.paid","data":{"object":{}}}`)
	if _, err := p.ParseWebhook(headers(sign(t, other, time.Now())), other); err != billing.ErrEventIgnored {
		t.Fatalf("want ErrEventIgnored, got %v", err)
	}
	// 删除事件 → 取消
	del := []byte(`{"id":"evt_d","type":"customer.subscription.deleted","data":{"object":{"id":"sub_x","metadata":{"user_id":"9"}}}}`)
	ev, err := p.ParseWebhook(headers(sign(t, del, time.Now())), del)
	if err != nil || ev.Type != billing.EventSubscriptionCanceled || ev.UserID != 9 {
		t.Fatalf("deleted event: %v %+v", err, ev)
	}
}
