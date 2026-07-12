package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ai-gateway/internal/config"
	"ai-gateway/internal/email"
	"ai-gateway/internal/model"
)

type AuthHandler struct{}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Language    string `json:"language"`
}

type updateUsernameRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required"`
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required"`
}

type resetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"email":        user.Email,
			"role":         user.Role,
			"time_zone":    user.TimeZone,
			"language":     user.Language,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) Me(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"email":        user.Email,
			"role":         user.Role,
			"time_zone":    user.TimeZone,
			"language":     user.Language,
		},
	})
}

// updateTimeZoneRequest 更新用户时区请求
type updateTimeZoneRequest struct {
	TimeZone string `json:"time_zone" binding:"required"`
}

// UpdateTimeZone 更新当前用户的时区偏好。
// 时区为 IANA 时区名（如 "Asia/Shanghai"、"America/New_York"、"UTC"）。
// 空字符串或无效时区返回错误。
func (h *AuthHandler) UpdateTimeZone(c *gin.Context) {
	var req updateTimeZoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证时区有效性
	if _, err := time.LoadLocation(req.TimeZone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time zone: " + err.Error()})
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := model.DB.Model(&user).Update("time_zone", req.TimeZone).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update time zone"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "time zone updated",
		"time_zone": req.TimeZone,
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid old password"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	if err := model.DB.Model(&user).Update("password_hash", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed"})
}

// UpdateProfile 更新用户资料：显示名称、邮箱、语言偏好。
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 邮箱校验：不能与其他用户冲突
	if req.Email != "" {
		if strings.Contains(req.Email, "@") {
			// 基本格式校验
			var existing model.User
			if err := model.DB.Where("email = ? AND id != ?", req.Email, user.ID).First(&existing).Error; err == nil {
				c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}
	}

	// 语言校验
	if req.Language != "" && req.Language != "zh" && req.Language != "en" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "language must be 'zh' or 'en'"})
		return
	}

	updates := map[string]interface{}{}
	if req.DisplayName != "" {
		updates["display_name"] = req.DisplayName
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.Language != "" {
		updates["language"] = req.Language
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	if err := model.DB.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "profile updated",
		"display_name": user.DisplayName,
		"email":        user.Email,
		"language":     user.Language,
	})
}

// UpdateUsername 修改用户名（需提供当前密码验证）。
func (h *AuthHandler) UpdateUsername(c *gin.Context) {
	var req updateUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid password"})
		return
	}

	// 检查用户名是否已存在
	var existing model.User
	if err := model.DB.Where("username = ? AND id != ?", req.Username, user.ID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	if err := model.DB.Model(&user).Update("username", req.Username).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update username"})
		return
	}

	// 更新 session
	session.Set("username", req.Username)
	session.Save()

	c.JSON(http.StatusOK, gin.H{"message": "username updated", "username": req.Username})
}

// ForgotPassword 发送密码重置邮件。
// 即使邮箱不存在也不报错，防止用户枚举攻击。
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// 邮箱未注册：不报错，防止用户枚举
		c.JSON(http.StatusOK, gin.H{"message": "if the email is registered, a reset link has been sent"})
		return
	}

	// 生成随机 token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// 设置过期时间（30分钟）
	expiry := time.Now().Add(30 * time.Minute)

	if err := model.DB.Model(&user).Updates(map[string]interface{}{
		"reset_token":        token,
		"reset_token_expiry": expiry,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save reset token"})
		return
	}

	// 立即返回成功，邮件异步发送以防止基于响应时间的用户枚举攻击
	c.JSON(http.StatusOK, gin.H{"message": "if the email is registered, a reset link has been sent"})

	// 异步发送邮件
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", getFrontendURL(), token)

	// 如果开启了日志记录重置链接，无论 SMTP 是否可用都打印
	if config.Get().SMTP.LogResetLink {
		log.Printf("[Auth] 🔑 Password reset link for %s (%s): %s", user.Username, user.Email, resetLink)
	}
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	emailAddr := user.Email

	go func() {
		if err := email.SendPasswordReset(emailAddr, displayName, resetLink); err != nil {
			log.Printf("[Auth] Failed to send reset email to %s: %v", emailAddr, err)
		} else {
			log.Printf("[Auth] Password reset email sent to %s", emailAddr)
		}
	}()
}

// ResetPassword 通过 token 重置密码。
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.User
	if err := model.DB.Where("reset_token = ?", req.Token).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
		return
	}

	// 检查是否过期
	if user.ResetTokenExpiry != nil && time.Now().After(*user.ResetTokenExpiry) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token has expired"})
		return
	}

	// 哈希新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	// 更新密码并清除重置 token
	if err := model.DB.Model(&user).Updates(map[string]interface{}{
		"password_hash":      string(hashedPassword),
		"reset_token":        "",
		"reset_token_expiry": nil,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}

	log.Printf("[Auth] Password reset successful for user %s", user.Username)
	c.JSON(http.StatusOK, gin.H{"message": "password has been reset"})
}

// getFrontendURL 返回前端基础 URL，用于生成密码重置链接。
// 优先使用 AG_SERVER_DOMAIN 环境变量；否则使用 localhost:端口。
func getFrontendURL() string {
	cfg := config.Get()
	if cfg != nil && cfg.Server.Domain != "" {
		return cfg.Server.Domain
	}
	if cfg != nil {
		port := cfg.Server.Port
		if port > 0 {
			return fmt.Sprintf("http://localhost:%d", port)
		}
	}
	return "http://localhost:18080"
}
