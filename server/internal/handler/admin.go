package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"

	"ai-gateway/internal/config"
	"ai-gateway/internal/email"
)

// AdminHandler 管理员操作（配置热加载等）
type AdminHandler struct{}

// ReloadConfig 触发配置热重载。
// POST /api/v1/admin/reload-config
func (h *AdminHandler) ReloadConfig(c *gin.Context) {
	if err := config.Reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reload failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "config reloaded"})
}

// GetSMTP 获取当前 SMTP 配置（密码脱敏）。
// GET /api/v1/admin/smtp
func (h *AdminHandler) GetSMTP(c *gin.Context) {
	cfg := config.Get()
	smtpCfg := cfg.SMTP

	// 密码脱敏：已保存则显示占位符
	pwdHint := ""
	if smtpCfg.Password != "" {
		pwdHint = "••••••••"
	}

	c.JSON(http.StatusOK, gin.H{
		"smtp": gin.H{
			"enabled":  smtpCfg.Enabled,
			"host":     smtpCfg.Host,
			"port":     smtpCfg.Port,
			"username": smtpCfg.Username,
			"password": pwdHint,
			"from":     smtpCfg.From,
			"use_tls":  smtpCfg.UseTLS,
		},
	})
}

type smtpSaveRequest struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

// SaveSMTP 保存 SMTP 配置到 config.yaml 并重新加载。
// POST /api/v1/admin/smtp
func (h *AdminHandler) SaveSMTP(c *gin.Context) {
	var req smtpSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := config.Get()

	// 检查旧密码占位符：如果传的是脱敏占位符，保留旧密码
	password := req.Password
	if password == "••••••••" && cfg.SMTP.Password != "" {
		password = cfg.SMTP.Password
	}

	// 写入 config.yaml
	if err := writeSMTPToYAML("config.yaml", req.Enabled, req.Host, req.Port,
		req.Username, password, req.From, req.UseTLS); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 热重载
	if err := config.Reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "配置已保存但重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMTP 配置已保存并生效", "success": true})
}

type smtpTestRequest struct {
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
	To       string `json:"to" binding:"required"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// TestSMTP 使用提供的 SMTP 配置发送测试邮件到指定收件人。
// POST /api/v1/admin/smtp/test
func (h *AdminHandler) TestSMTP(c *gin.Context) {
	var req smtpTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.To == "" || !strings.Contains(req.To, "@") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的收件人邮箱"})
		return
	}

	// 如果密码是脱敏占位符，回退使用服务器已保存的密码
	cfg := config.Get()
	password := req.Password
	if password == "••••••••" && cfg.SMTP.Password != "" {
		password = cfg.SMTP.Password
	}

	email.InitDirect(email.Config{
		Enabled:  true,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: password,
		From:     req.From,
		UseTLS:   req.UseTLS,
	})

	subject := req.Subject
	if subject == "" {
		subject = "AI Gateway - SMTP 测试邮件"
	}
	body := req.Body
	if body == "" {
		body = "<p>这是一封来自 AI Gateway 的 SMTP 测试邮件。</p><p>如果您收到此邮件，说明 SMTP 配置正确。</p>"
	}

	err := email.SendCustom([]string{req.To}, subject, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "SMTP 测试失败: " + err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "测试邮件发送成功，请检查收件箱",
		"success": true,
	})
}

// writeSMTPToYAML 读取 config.yaml，更新 smtp 配置段后写回。
// 使用 YAML 反序列化/序列化保留原有结构。
func writeSMTPToYAML(path string, enabled bool, host string, port int,
	username, password, from string, useTLS bool) error {

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// 解析为通用 map
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	// 确保 smtp 键存在
	smtp, ok := root["smtp"].(map[string]interface{})
	if !ok {
		smtp = make(map[string]interface{})
		root["smtp"] = smtp
	}

	smtp["enabled"] = enabled
	smtp["host"] = host
	smtp["port"] = port
	smtp["username"] = username
	smtp["password"] = password
	smtp["from"] = from
	smtp["use_tls"] = useTLS

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0644)
}
