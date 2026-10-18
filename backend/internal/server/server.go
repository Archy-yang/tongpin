// Package server 装配层：把各业务域的依赖组装起来并注册路由。
// 域之间的依赖关系只在这里出现，域内部互相不可见。
package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Archy-yang/tongpin/backend/internal/analytics"
	"github.com/Archy-yang/tongpin/backend/internal/config"
	"github.com/Archy-yang/tongpin/backend/internal/domain/auth"
	"github.com/Archy-yang/tongpin/backend/internal/domain/billing"
	"github.com/Archy-yang/tongpin/backend/internal/domain/chat"
	"github.com/Archy-yang/tongpin/backend/internal/domain/consent"
	"github.com/Archy-yang/tongpin/backend/internal/domain/contact"
	"github.com/Archy-yang/tongpin/backend/internal/domain/icebreak"
	"github.com/Archy-yang/tongpin/backend/internal/domain/match"
	"github.com/Archy-yang/tongpin/backend/internal/domain/safety"
	"github.com/Archy-yang/tongpin/backend/internal/domain/style"
	"github.com/Archy-yang/tongpin/backend/internal/llm"
	"github.com/Archy-yang/tongpin/backend/internal/payments/stripe"
	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/mailer"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
	"github.com/Archy-yang/tongpin/backend/internal/realtime"
)

// llmCompleter 把 llm.Provider 适配为 style.Completer（system+user 两段式）。
type llmCompleter struct{ p llm.Provider }

func (a llmCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	return a.p.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3, // 分析任务要稳定，温度低于聊天
	})
}

// chatMessageSource 把 chat.Service 适配为 style.ChatMessageSource。
type chatMessageSource struct{ chat *chat.Service }

func (a chatMessageSource) MessagesSince(ctx context.Context, userID, afterID int64, limit int) ([]style.SourceMessage, error) {
	msgs, err := a.chat.MessagesSince(ctx, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]style.SourceMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, style.SourceMessage{ID: m.ID, Sender: m.Sender, Content: m.Content, CreatedAt: m.CreatedAt})
	}
	return out, nil
}

func (a chatMessageSource) CountMessages(ctx context.Context, userID int64) (int, error) {
	return a.chat.CountMessages(ctx, userID)
}

type Server struct {
	cfg    *config.Config
	db     *pgxpool.Pool
	router *gin.Engine

	styleSvc *style.Service
	chatSvc  *chat.Service
}

func New(cfg *config.Config, db *pgxpool.Pool) *Server {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	s := &Server{cfg: cfg, db: db, router: r}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	issuer := jwt.NewIssuer(s.cfg.JWTSecret)
	authMw := middleware.Auth(issuer)

	api := s.router.Group("/api/v1")

	// WebSocket 实时推送（自带查询参数鉴权，不走 Auth 中间件）
	hub := realtime.NewHub()
	s.router.GET("/api/v1/ws", realtime.Serve(hub, issuer))

	// 公开路由
	authSvc := auth.NewService(auth.NewRepository(s.db), issuer, mailer.New(mailer.FromEnv()))
	authHandler := auth.NewHandler(authSvc)
	authHandler.RegisterPublicRoutes(api)
	authHandler.RegisterEmailRoutes(api)

	// 登录态路由
	authed := api.Group("", authMw)
	authHandler.RegisterRoutes(authed)

	// chat 域：AI 陪伴聊天
	provider := llm.NewOpenAICompat(s.cfg.LLM.BaseURL, s.cfg.LLM.APIKey, s.cfg.LLM.Model)

	// 埋点：LLM 用量在 provider 咽喉处统一记录（归属用户经 ctx 带入）；
	// 各域行为事件（发消息/暖场/真人消息）由域内 Record。
	rec := analytics.New(s.db)
	provider.SetUsageHook(func(ctx context.Context, u llm.Usage) {
		rec.Record(ctx, llm.UserIDFrom(ctx), analytics.EvLLMCall, map[string]any{
			"in_tokens": u.PromptTokens, "out_tokens": u.CompletionTokens, "model": s.cfg.LLM.Model,
		})
	})

	chatRepo := chat.NewRepository(s.db)
	chatSvc := chat.NewService(chatRepo, provider, nil) // style 尚未创建，下面回填风格对齐
	s.chatSvc = chatSvc
	chatSvc.SetFreeQuotaDefault(s.cfg.FreeDailyMsgQuota)
	chatSvc.SetRecorder(rec)
	chatSvc.SetNotifier(chatNotifier{hub: hub})
	chatHandler := chat.NewHandler(chatSvc)
	chatHandler.RegisterRoutes(authed)

	// style 域：异步风格分析 + 风格卡
	styleRepo := style.NewRepository(s.db)
	s.styleSvc = style.NewService(styleRepo, chatMessageSource{chat: chatSvc}, llmCompleter{p: provider})
	style.NewHandler(s.styleSvc).RegisterRoutes(authed)

	// 回填：chat 的风格对齐来自 style 域（chat 与 style 互不 import，经接口解耦）
	chatSvc.SetStyleHints(styleHintProvider{style: s.styleSvc})

	// 安全域（match/contact 依赖其拉黑检查）
	safetySvc := safety.NewService(safety.NewRepository(s.db))
	safety.NewHandler(safetySvc).RegisterRoutes(authed)

	// match 域：匹配池 + 契合提示（authSvc 已在公开路由处创建，跨域用户信息直接复用）
	// consent 与 match 互相依赖（match 记授权、consent 撤回时关 match），
	// 用延迟绑定的记录器打破循环：HTTP 请求发生在装配完成之后，svc 必然已就绪。
	consentRec := &deferredConsent{}
	matchSvc := match.NewService(
		match.NewRepository(s.db),
		authDirectory{auth: authSvc},
		safetySvc,
		consentRec,
		s.cfg.MatchPoolThreshold,
	)

	// consent 域：授权审计（撤回 real_contact 时关闭 match）
	consentSvc := consent.NewService(consent.NewRepository(s.db), matchSvc)
	consentRec.svc = consentSvc

	// contact 域：真人接触 + 一对一聊天
	contactSvc := contact.NewService(
		contact.NewRepository(s.db),
		matchSvc,
		safetySvc,
		consentSvc,
		authNamer{auth: authSvc},
	)
	contactSvc.SetNotifier(hubAdapter{hub: hub})
	contactSvc.SetRecorder(rec)
	contact.NewHandler(contactSvc).RegisterRoutes(authed)
	match.NewHandler(matchSvc).RegisterRoutes(authed)
	consent.NewHandler(consentSvc).RegisterRoutes(authed)

	// icebreak 域：AI 暖场（双方同意后双代理试聊 + 破冰摘要）
	icebreakSvc := icebreak.NewService(
		icebreak.NewRepository(s.db),
		matchSvc,
		consentSvc,
		s.styleSvc,
		llmCompleter{p: provider},
	)
	icebreakSvc.SetRecorder(rec)
	icebreak.NewHandler(icebreakSvc).RegisterRoutes(authed)

	// billing 域：订阅与支付（渠道适配器见 internal/payments，按配置注册）
	// 只配了 webhook 密钥也能注册——收银台在 CreateCheckout 内部自查密钥，
	// 而 webhook 接收（如测试/灰度先行）只需要验签密钥。
	billingSvc := billing.NewService(billing.NewRepository(s.db))
	if s.cfg.Stripe.SecretKey != "" || s.cfg.Stripe.WebhookSecret != "" {
		billingSvc.Register(stripe.New(stripe.Config{
			SecretKey:     s.cfg.Stripe.SecretKey,
			WebhookSecret: s.cfg.Stripe.WebhookSecret,
			PriceID:       s.cfg.Stripe.PriceMonthly,
			SuccessURL:    s.cfg.Stripe.PublicBaseURL + "/me?billing=success",
			CancelURL:     s.cfg.Stripe.PublicBaseURL + "/me?billing=cancel",
		}))
	}
	billing.NewHandler(billingSvc).RegisterRoutes(authed)
	billing.NewHandler(billingSvc).RegisterWebhookRoutes(api) // 公开路由：验签即鉴权

	// 会员权益接入 chat 额度（chat 不感知 billing，经消费方接口回填）
	chatSvc.SetEntitlements(billingEntitlements{billing: billingSvc})

	// 埋点上报与管理指标（app_open 走登录态；/admin/metrics 凭 X-Admin-Token）
	authed.POST("/analytics/open", func(c *gin.Context) {
		rec.Record(c.Request.Context(), middleware.UserIDFrom(c), analytics.EvAppOpen, nil)
		httpx.OK(c, nil)
	})
	analytics.NewAdminHandler(rec, s.cfg.AdminToken).RegisterAdminRoutes(api)
}

// ---- 跨域适配器（依赖接线只出现在装配层）----

// authDirectory 把 auth.Service 适配为 match.UserDirectory。
type authDirectory struct{ auth *auth.Service }

func (a authDirectory) Basic(ctx context.Context, userID int64) (match.BasicUser, error) {
	b, err := a.auth.Basic(ctx, userID)
	if err != nil {
		return match.BasicUser{}, err
	}
	return match.BasicUser{
		Nickname: b.Nickname, Gender: b.Gender, GenderPref: b.GenderPref,
		AgeRange: b.AgeRange, City: b.City,
	}, nil
}

// styleHintProvider 把 style.Service 适配为 chat.StyleHintProvider（AI 回复风格对齐）。
type styleHintProvider struct{ style *style.Service }

func (a styleHintProvider) Hint(ctx context.Context, userID int64) (chat.StyleHint, bool, error) {
	raw, confidence, ok, err := a.style.LatestAnalysis(ctx, userID)
	if err != nil || !ok {
		return chat.StyleHint{}, false, err
	}
	var p struct {
		CommunicationStyle struct {
			ReplyLength string  `json:"reply_length"`
			Formality   string  `json:"formality"`
			EmojiUsage  string  `json:"emoji_usage"`
			Pace        string  `json:"pace"`
			Initiative  float64 `json:"initiative"`
		} `json:"communication_style"`
		EmotionalStyle struct {
			Tone string `json:"tone"`
		} `json:"emotional_style"`
		ActiveHours []string `json:"active_hours"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return chat.StyleHint{}, false, nil // 画像解析失败时降级为基础人格
	}
	return chat.StyleHint{
		ReplyLength: p.CommunicationStyle.ReplyLength,
		Formality:   p.CommunicationStyle.Formality,
		EmojiUsage:  p.CommunicationStyle.EmojiUsage,
		Tone:        p.EmotionalStyle.Tone,
		Pace:        p.CommunicationStyle.Pace,
		ActiveHours: p.ActiveHours,
		Confidence:  confidence,
	}, true, nil
}

// hubAdapter 把 realtime.Hub 适配为 contact.RealtimePublisher。
type hubAdapter struct{ hub *realtime.Hub }

func (a hubAdapter) PublishDirectMessage(_ context.Context, matchID int64, msg contact.DirectMessage, toUsers ...int64) {
	a.hub.Broadcast(toUsers, realtime.Frame{Type: "dm", MatchID: matchID, Message: msg})
}

// chatNotifier 把 realtime.Hub 适配为 chat.AINotifier（主动消息实时推送）。
type chatNotifier struct{ hub *realtime.Hub }

func (a chatNotifier) NotifyAI(_ context.Context, userID, conversationID int64, msg chat.MessageDTO) {
	a.hub.Broadcast([]int64{userID}, realtime.Frame{Type: "chat_ai", Message: msg})
}

// authNamer 把 auth.Service 适配为 contact.UserDirectory。
type authNamer struct{ auth *auth.Service }

func (a authNamer) Nickname(ctx context.Context, userID int64) (string, error) {
	return a.auth.Nickname(ctx, userID)
}

// billingEntitlements 把 billing.Service 适配为 chat.EntitlementProvider（会员不限量）。
type billingEntitlements struct{ billing *billing.Service }

func (b billingEntitlements) IsEntitled(ctx context.Context, userID int64) (bool, error) {
	return b.billing.Entitlement(ctx, userID)
}

// deferredConsent 把 consent.Service 适配为 match.ConsentRecorder / contact.ConsentRecorder，
// 以延迟绑定解决 match ↔ consent 的装配循环。
type deferredConsent struct{ svc *consent.Service }

func (d *deferredConsent) Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error {
	return d.svc.Record(ctx, userID, targetType, targetID, granted)
}

// Start 启动后台任务（风格分析 / 长期记忆 / 主动开话三个 worker）。ctx 结束时退出。
func (s *Server) Start(ctx context.Context) {
	interval := time.Duration(s.cfg.WorkerIntervalSec) * time.Second
	go style.NewWorker(s.styleSvc, interval).Run(ctx)
	go chat.NewMemoryWorker(s.chatSvc, interval).Run(ctx)
	go chat.NewProactiveWorker(s.chatSvc, s.proactiveInterval()).Run(ctx)
}

func (s *Server) proactiveInterval() time.Duration {
	return time.Duration(s.cfg.ProactiveIntervalSec) * time.Second
}

func (s *Server) Run() error {
	return s.router.Run(":" + s.cfg.Port)
}
