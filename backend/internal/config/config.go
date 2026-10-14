// Package config 加载应用配置。MVP 阶段只从环境变量读取，不引入配置库。
package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env              string // dev | prod
	Port             string
	DatabaseURL      string
	JWTSecret        string
	MatchPoolThreshold int // 契合提示灰度阈值：池内活跃用户低于此值不开匹配（本地测试调低）
	LLM              LLMConfig
}

type LLMConfig struct {
	BaseURL string // OpenAI 兼容接口地址，如 https://api.deepseek.com/v1
	APIKey  string
	Model   string
}

func Load() *Config {
	threshold := 500
	if v := os.Getenv("MATCH_POOL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			threshold = n
		}
	}
	return &Config{
		Env:                getenv("APP_ENV", "dev"),
		Port:               getenv("APP_PORT", "8080"),
		DatabaseURL:        getenv("DATABASE_URL", "postgres://tongpin:tongpin@localhost:5432/tongpin?sslmode=disable"),
		JWTSecret:          getenv("JWT_SECRET", "dev-secret-change-me"),
		MatchPoolThreshold: threshold,
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
