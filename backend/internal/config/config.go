// Package config 加载应用配置。MVP 阶段只从环境变量读取，不引入配置库。
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Env                  string // dev | prod
	Port                 string
	DatabaseURL          string
	JWTSecret            string
	MatchPoolThreshold   int // 契合提示灰度阈值：池内活跃用户低于此值不开匹配（本地测试调低）
	WorkerIntervalSec    int // 风格/记忆 worker 轮询间隔（秒），默认 300
	ProactiveIntervalSec int // 主动开话 worker 轮询间隔（秒），默认 900
	FreeDailyMsgQuota    int // 免费用户每日 AI 聊天条数（0 或负数 = 关闭限制），默认 10
	BoundDailyMsgQuota   int // 绑定邮箱后的每日条数（注册激励），默认 15
	SignupIPDailyLimit   int // 每 IP 每日最多创建的新设备账号数（0 或负数 = 关闭），默认 10
	AdminToken           string // 管理端点令牌（X-Admin-Token）；空 = 管理端点关闭
	Stripe               StripeConfig
	LLM                  LLMConfig
}

type StripeConfig struct {
	SecretKey     string // sk_test_... 留空 = 支付未开放
	WebhookSecret string // whsec_...
	PriceMonthly  string // price_... 会员月度价格 ID
	PublicBaseURL string // 成功/取消跳转的网页基地址
}

type LLMConfig struct {
	BaseURL string // OpenAI 兼容接口地址，如 https://api.deepseek.com/v1
	APIKey  string
	Model   string
}

func Load() *Config {
	loadDotEnv()
	threshold := 500
	if v := os.Getenv("MATCH_POOL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			threshold = n
		}
	}
	return &Config{
		Env:                  getenv("APP_ENV", "dev"),
		Port:                 getenv("APP_PORT", "8080"),
		DatabaseURL:          getenv("DATABASE_URL", "postgres://tongpin:tongpin@localhost:15432/tongpin?sslmode=disable"),
		JWTSecret:            getenv("JWT_SECRET", "dev-secret-change-me"),
		MatchPoolThreshold:   threshold,
		WorkerIntervalSec:    workerInterval(),
		ProactiveIntervalSec: proactiveInterval(),
		FreeDailyMsgQuota:    freeDailyQuota(),
		BoundDailyMsgQuota:   boundDailyQuota(),
		SignupIPDailyLimit:   signupIPDailyLimit(),
		AdminToken:           getenv("ADMIN_TOKEN", ""),
		Stripe: StripeConfig{
			SecretKey:     getenv("STRIPE_SECRET_KEY", ""),
			WebhookSecret: getenv("STRIPE_WEBHOOK_SECRET", ""),
			PriceMonthly:  getenv("STRIPE_PRICE_MONTHLY", ""),
			PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:5174"),
		},
		LLM: LLMConfig{
			BaseURL: getenv("LLM_BASE_URL", "https://api.deepseek.com/v1"),
			APIKey:  getenv("LLM_API_KEY", ""),
			Model:   getenv("LLM_MODEL", "deepseek-chat"),
		},
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func workerInterval() int {
	if v := os.Getenv("STYLE_WORKER_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 300
}

func proactiveInterval() int {
	if v := os.Getenv("PROACTIVE_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 900
}

func freeDailyQuota() int {
	if v := os.Getenv("FREE_DAILY_MSG_QUOTA"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 10
}

func boundDailyQuota() int {
	if v := os.Getenv("BOUND_DAILY_MSG_QUOTA"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 15
}

func signupIPDailyLimit() int {
	if v := os.Getenv("SIGNUP_IP_DAILY_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 10
}

// loadDotEnv 加载 .env（当前目录或上级目录），已有环境变量优先。
func loadDotEnv() {
	candidates := []string{".env", filepath.Join("..", ".env")}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			line := strings.TrimSpace(scan.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
		return // 只加载找到的第一个
	}
}
