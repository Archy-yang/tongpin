package analytics

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
)

// DailyRow 单日聚合（成本/活跃侧）。
type DailyRow struct {
	Day         string `json:"day"`
	ActiveUsers int    `json:"active_users"` // 当日有任何行为的用户数
	NewUsers    int    `json:"new_users"`
	Messages    int    `json:"messages"`   // 用户发的 AI 陪伴消息数
	LLMCalls    int    `json:"llm_calls"`
	InTokens    int64  `json:"in_tokens"`
	OutTokens   int64  `json:"out_tokens"`
	Icebreaks   int    `json:"icebreaks"`
	DMSent      int    `json:"dm_sent"`
}

// RetentionRow 单日活跃用户的次日/7日回访（活跃留存，非注册留存）。
type RetentionRow struct {
	Day    string `json:"day"`
	Active int    `json:"active"`
	D1     int    `json:"d1"`
	D7     int    `json:"d7"`
}

// DailySummary 最近 days 天的每日聚合。
func (r *Recorder) DailySummary(ctx context.Context, days int) ([]DailyRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.created_at::date,
		       COUNT(DISTINCT e.user_id),
		       COALESCE(u.new_users, 0),
		       COUNT(*) FILTER (WHERE e.name = $1),
		       COUNT(*) FILTER (WHERE e.name = $2),
		       COALESCE(SUM((e.props->>'in_tokens')::bigint) FILTER (WHERE e.name = $2), 0),
		       COALESCE(SUM((e.props->>'out_tokens')::bigint) FILTER (WHERE e.name = $2), 0),
		       COUNT(*) FILTER (WHERE e.name = $3),
		       COUNT(*) FILTER (WHERE e.name = $4)
		FROM analytics_events e
		LEFT JOIN (
			SELECT created_at::date AS day, COUNT(*) AS new_users
			FROM users WHERE created_at >= CURRENT_DATE - $5::int
			GROUP BY 1
		) u ON u.day = e.created_at::date
		WHERE e.created_at >= CURRENT_DATE - $5::int
		GROUP BY e.created_at::date, u.new_users
		ORDER BY e.created_at::date
	`, EvChatUserMsg, EvLLMCall, EvIcebreakCreate, EvDMSent, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyRow
	for rows.Next() {
		var day time.Time
		var row DailyRow
		if err := rows.Scan(&day, &row.ActiveUsers, &row.NewUsers, &row.Messages,
			&row.LLMCalls, &row.InTokens, &row.OutTokens, &row.Icebreaks, &row.DMSent); err != nil {
			return nil, err
		}
		row.Day = day.Format("2006-01-02")
		out = append(out, row)
	}
	return out, rows.Err()
}

// Retention 最近 days 天的活跃留存（当天活跃的用户在 +1/+7 天是否再来）。
func (r *Recorder) Retention(ctx context.Context, days int) ([]RetentionRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.day,
		       COUNT(DISTINCT a.user_id),
		       COUNT(DISTINCT n1.user_id),
		       COUNT(DISTINCT n7.user_id)
		FROM (
			SELECT DISTINCT user_id, created_at::date AS day
			FROM analytics_events
			WHERE created_at >= CURRENT_DATE - $1::int
		) a
		LEFT JOIN analytics_events n1
		       ON n1.user_id = a.user_id AND n1.created_at::date = a.day + 1
		LEFT JOIN analytics_events n7
		       ON n7.user_id = a.user_id AND n7.created_at::date = a.day + 7
		GROUP BY a.day ORDER BY a.day
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RetentionRow
	for rows.Next() {
		var day time.Time
		var row RetentionRow
		if err := rows.Scan(&day, &row.Active, &row.D1, &row.D7); err != nil {
			return nil, err
		}
		row.Day = day.Format("2006-01-02")
		out = append(out, row)
	}
	return out, rows.Err()
}

// AdminHandler 只读管理端点：X-Admin-Token 守卫，token 未配置则整体关闭。
type AdminHandler struct {
	rec   *Recorder
	token string
}

func NewAdminHandler(rec *Recorder, token string) *AdminHandler {
	return &AdminHandler{rec: rec, token: token}
}

// RegisterAdminRoutes 挂在公开 api 组上（凭 token 自证身份，不走登录态）。
func (h *AdminHandler) RegisterAdminRoutes(rg gin.IRouter) {
	rg.GET("/admin/metrics", h.metrics)
}

func (h *AdminHandler) metrics(c *gin.Context) {
	if h.token == "" || c.GetHeader("X-Admin-Token") != h.token {
		httpx.Fail(c, 404, httpx.CodeNotFound, "not found")
		return
	}
	days := 30
	if v, err := strconv.Atoi(c.Query("days")); err == nil && v > 0 && v <= 90 {
		days = v
	}
	daily, err := h.rec.DailySummary(c.Request.Context(), days)
	if err != nil {
		httpx.FailInternal(c, "metrics query failed")
		return
	}
	retention, err := h.rec.Retention(c.Request.Context(), days)
	if err != nil {
		httpx.FailInternal(c, "retention query failed")
		return
	}
	httpx.OK(c, gin.H{"daily": daily, "retention": retention})
}
