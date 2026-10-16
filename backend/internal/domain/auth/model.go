package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// User 领域模型。设备匿名登录为主，邮箱 + 密码为正式凭证（可后绑定升级）。
type User struct {
	ID           int64
	DeviceID     *string
	Nickname     string
	Email        *string
	PasswordHash *string
	Status       string
	CreatedAt    time.Time
}

type DeviceLoginRequest struct {
	DeviceID string `json:"device_id" binding:"required,max=64"`
}

type UpdateBasicsRequest struct {
	Nickname   string `json:"nickname" binding:"omitempty,max=32"`
	AgeRange   string `json:"age_range" binding:"omitempty,oneof=18-24 25-30 31-40 40+"`
	Gender     string `json:"gender" binding:"omitempty,oneof=male female other"`
	City       string `json:"city" binding:"omitempty,max=32"`
	GenderPref string `json:"gender_pref" binding:"omitempty,oneof=male female other"`
}

type LoginResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
	IsNew bool    `json:"is_new"`
}

type UserDTO struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
}

// GenerateNickname 生成匿名昵称，如 "同频_a3f9c2"。
func GenerateNickname() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return "同频_" + hex.EncodeToString(b)
}
