// Package mailer 邮件发送：SMTP 实现 + 开发模式（打日志）。
// 通过 SMTP_HOST 等环境变量配置；未配置时退回 Dev 模式，验证码直接进日志。
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, textBody string) error
}

type Config struct {
	Host     string // 如 smtp.qq.com
	Port     string // 如 465（隐式 TLS）或 587（STARTTLS）
	Username string
	Password string // 授权码，非登录密码
	From     string // 发件人，如 "同频 <xxx@qq.com>"
}

func FromEnv() Config {
	return Config{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     getenv("SMTP_PORT", "465"),
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     getenv("SMTP_FROM", os.Getenv("SMTP_USERNAME")),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// New 按配置选择实现：无 SMTP 配置 → Dev 模式。
func New(cfg Config) Mailer {
	if cfg.Host == "" {
		return &DevMailer{}
	}
	return &SMTPMailer{cfg: cfg}
}

// DevMailer 开发模式：不真发，内容进日志（验证码可从日志取）。
type DevMailer struct{}

func (d *DevMailer) Send(_ context.Context, to, subject, body string) error {
	log.Printf("[mailer:dev] to=%s subject=%q body=%q", to, subject, body)
	return nil
}

type SMTPMailer struct {
	cfg Config
}

func (s *SMTPMailer) Send(_ context.Context, to, subject, body string) error {
	addr := s.cfg.Host + ":" + s.cfg.Port
	msg := strings.Join([]string{
		"From: " + s.cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	if s.cfg.Port == "465" { // 隐式 TLS
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.cfg.Host})
		if err != nil {
			return fmt.Errorf("smtp dial tls: %w", err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		if err := client.Mail(s.cfg.From); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}

	// STARTTLS（587）
	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg))
}
