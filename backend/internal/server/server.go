// Package server 装配层：把各业务域的依赖组装起来并注册路由。
// 域之间的依赖关系只在这里出现，域内部互相不可见。
package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Archy-yang/tongpin/backend/internal/config"
	"github.com/Archy-yang/tongpin/backend/internal/domain/auth"
	"github.com/Archy-yang/tongpin/backend/internal/domain/chat"
	"github.com/Archy-yang/tongpin/backend/internal/domain/consent"
	"github.com/Archy-yang/tongpin/backend/internal/domain/contact"
	"github.com/Archy-yang/tongpin/backend/internal/domain/match"
	"github.com/Archy-yang/tongpin/backend/internal/domain/safety"
	"github.com/Archy-yang/tongpin/backend/internal/domain/style"
	"github.com/Archy-yang/tongpin/backend/internal/llm"
	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
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

	// 公开路由
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(s.db), issuer))
	authHandler.RegisterPublicRoutes(api)

	// 登录态路由
	authed := api.Group("", authMw)
	authHandler.RegisterRoutes(authed)

	// chat 域：AI 陪伴聊天
	provider := llm.NewOpenAICompat(s.cfg.LLM.BaseURL, s.cfg.LLM.APIKey, s.cfg.LLM.Model)
	chatSvc := chat.NewService(chat.NewRepository(s.db), provider)
	chatHandler := chat.NewHandler(chatSvc)
	chatHandler.RegisterRoutes(authed)

	// style 域：异步风格分析 + 风格卡
	styleRepo := style.NewRepository(s.db)
	s.styleSvc = style.NewService(styleRepo, chatMessageSource{chat: chatSvc}, llmCompleter{p: provider})
	style.NewHandler(s.styleSvc).RegisterRoutes(authed)

	// 安全域（match/contact 依赖其拉黑检查）
	safetySvc := safety.NewService(safety.NewRepository(s.db))
	safety.NewHandler(safetySvc).RegisterRoutes(authed)

	// auth 提供跨域用户信息
	authRepo := auth.NewRepository(s.db)
	authSvc := auth.NewService(authRepo, issuer)

	// match 域：匹配池 + 契合提示
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
	contact.NewHandler(contactSvc).RegisterRoutes(authed)
	match.NewHandler(matchSvc).RegisterRoutes(authed)
	consent.NewHandler(consentSvc).RegisterRoutes(authed)
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

// authNamer 把 auth.Service 适配为 contact.UserDirectory。
type authNamer struct{ auth *auth.Service }

func (a authNamer) Nickname(ctx context.Context, userID int64) (string, error) {
	return a.auth.Nickname(ctx, userID)
}

// deferredConsent 把 consent.Service 适配为 match.ConsentRecorder / contact.ConsentRecorder，
// 以延迟绑定解决 match ↔ consent 的装配循环。
type deferredConsent struct{ svc *consent.Service }

func (d *deferredConsent) Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error {
	return d.svc.Record(ctx, userID, targetType, targetID, granted)
}

// Start 启动后台任务（风格分析 worker）。ctx 结束时 worker 退出。
func (s *Server) Start(ctx context.Context) {
	interval := time.Duration(s.cfg.WorkerIntervalSec) * time.Second
	worker := style.NewWorker(s.styleSvc, interval)
	go worker.Run(ctx)
}

func (s *Server) Run() error {
	return s.router.Run(":" + s.cfg.Port)
}
