package auth

// EmailCredentialsRequest 注册 / 绑定共用的请求体。
type EmailCredentialsRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type EmailLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,max=72"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,max=72"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=72"`
}

type ResetRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetConfirmRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=72"`
}

// UserDTO 扩展：邮箱用于前端判断账号形态（设备匿名 / 已绑定）。
// 原 UserDTO 保持不变，这里定义带邮箱的版本。
type AccountDTO struct {
	ID       int64   `json:"id"`
	Nickname string  `json:"nickname"`
	Email    *string `json:"email"` // null = 未绑定的设备匿名账号
}
