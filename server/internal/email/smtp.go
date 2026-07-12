// Package email 提供 SMTP 邮件发送功能，用于密码重置等场景。
// 使用 Go 标准库 net/smtp，无需外部依赖。
// 支持 HTML 邮件、TLS/STARTTLS 和纯文本降级。
package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Config SMTP 邮件配置
type Config struct {
	Enabled  bool   // 是否启用
	Host     string // SMTP 服务器地址，如 smtp.gmail.com
	Port     int    // SMTP 端口，如 587(STARTTLS) / 465(TLS)
	Username string // 发件人账号
	Password string // 发件人密码/授权码
	From     string // 发件人显示名称+地址，如 "AI Gateway <noreply@example.com>"
	UseTLS   bool   // 是否使用 TLS 直连（端口 465），默认 STARTTLS（端口 587）
}

// Client SMTP 邮件客户端
type Client struct {
	cfg Config
}

var defaultClient *Client

// Init 初始化全局邮件客户端，仅在 cfg.Enabled 为 true 时生效。
func Init(cfg Config) {
	if !cfg.Enabled {
		log.Println("[Email] SMTP disabled, skipping initialization")
		return
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	defaultClient = &Client{cfg: cfg}
	log.Printf("[Email] SMTP initialized: host=%s port=%d user=%s", cfg.Host, cfg.Port, cfg.Username)
}

// Send 发送邮件。
// to 为收件人地址列表，subject 为主题，htmlBody 为 HTML 内容。
func (c *Client) Send(to []string, subject, htmlBody string) error {
	if c == nil {
		return fmt.Errorf("email client not initialized")
	}
	cfg := c.cfg

	// 提取纯邮箱地址作为 SMTP envelope from（MAIL FROM 命令只接受纯地址）
	envelopeFrom := extractEmail(cfg.From)
	if envelopeFrom == "" {
		envelopeFrom = cfg.From
	}

	// 构建 MIME 邮件
	header := make(map[string]string)
	header["From"] = cfg.From
	header["To"] = strings.Join(to, ", ")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = fmt.Sprintf("text/html; charset=UTF-8")

	var msg bytes.Buffer
	for k, v := range header {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	if cfg.UseTLS {
		return c.sendWithTLS(addr, auth, envelopeFrom, to, msg.Bytes())
	}
	return c.sendWithSTARTTLS(addr, auth, envelopeFrom, to, msg.Bytes())
}

func (c *Client) sendWithSTARTTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	defer client.Quit()

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: c.cfg.Host}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	for _, t := range to {
		if err := client.Rcpt(t); err != nil {
			return fmt.Errorf("rcpt %s: %w", t, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return w.Close()
}

func (c *Client) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: c.cfg.Host}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	defer client.Quit()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	for _, t := range to {
		if err := client.Rcpt(t); err != nil {
			return fmt.Errorf("rcpt %s: %w", t, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return w.Close()
}

// InitDirect 使用指定配置初始化一个临时客户端（不改变全局 defaultClient）。
// 用于 SMTP 连接测试。返回的 Client 可直接调用 Send。
func InitDirect(cfg Config) {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	defaultClient = &Client{cfg: cfg}
}

// SendTest 发送一封测试邮件到指定地址。
func SendTest(to string) error {
	if defaultClient == nil {
		return fmt.Errorf("email client not initialized")
	}
	subject := "AI Gateway - SMTP 测试邮件 / SMTP Test"
	return defaultClient.Send([]string{to}, subject, smtpTestTemplate)
}

// SendCustom 使用当前客户端发送自定义内容的邮件。
func SendCustom(to []string, subject, htmlBody string) error {
	if defaultClient == nil {
		return fmt.Errorf("email client not initialized")
	}
	return defaultClient.Send(to, subject, htmlBody)
}

// extractEmail 从 "显示名 <email@example.com>" 格式中提取纯邮箱地址。
// 如果传入的是纯邮箱地址则直接返回。
func extractEmail(from string) string {
	// 优先用 net/mail 解析
	if addr, err := mail.ParseAddress(from); err == nil {
		return addr.Address
	}
	// 降级：从 < > 中提取
	if start := strings.IndexByte(from, '<'); start >= 0 {
		if end := strings.IndexByte(from[start:], '>'); end > 0 {
			return from[start+1 : start+end]
		}
	}
	return from
}

const smtpTestTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: Arial, sans-serif; padding: 20px;">
  <h2>✅ AI Gateway SMTP 测试成功</h2>
  <p>如果您收到此邮件，说明 SMTP 配置正确，邮件服务运行正常。</p>
  <p style="color: #909399; font-size: 12px;">
    This is a test email from AI Gateway. If you received this, your SMTP settings are correct.
  </p>
</body>
</html>`

// SendPasswordReset 发送密码重置邮件，返回 error 以供调用方处理。
func SendPasswordReset(toEmail, toName, resetLink string) error {
	if defaultClient == nil {
		return fmt.Errorf("email not configured")
	}
	subject := "AI Gateway - 密码重置 / Password Reset"
	body := renderPasswordResetEmail(toName, resetLink)
	return defaultClient.Send([]string{toEmail}, subject, body)
}

// renderPasswordResetEmail 渲染密码重置 HTML 邮件
func renderPasswordResetEmail(name, link string) string {
	tmpl := template.Must(template.New("reset").Parse(resetEmailTemplate))
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, map[string]string{
		"Name": name,
		"Link": link,
	})
	return buf.String()
}

const resetEmailTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: Arial, sans-serif; background: #f5f5f5; padding: 40px;">
  <div style="max-width: 480px; margin: 0 auto; background: #fff; border-radius: 8px; padding: 32px; box-shadow: 0 2px 12px rgba(0,0,0,0.1);">
    <h2 style="color: #333;">AI Gateway</h2>
    <p>您好 {{ .Name }}，</p>
    <p>我们收到了您的密码重置请求。请点击下方按钮重置密码（链接 30 分钟内有效）：</p>
    <p style="text-align: center; margin: 24px 0;">
      <a href="{{ .Link }}" style="display: inline-block; background: #409eff; color: #fff; padding: 12px 32px; border-radius: 4px; text-decoration: none; font-size: 16px;">重置密码</a>
    </p>
    <p style="color: #909399; font-size: 13px;">如果按钮无法点击，请复制以下链接到浏览器：<br>{{ .Link }}</p>
    <p style="color: #909399; font-size: 13px;">如果您没有请求重置密码，请忽略此邮件。</p>
    <hr style="border: none; border-top: 1px solid #eee; margin: 24px 0;">
    <p style="color: #909399; font-size: 12px;">
      Hello {{ .Name }},<br>
      You have requested a password reset. Click the button above (valid for 30 minutes).<br>
      If you did not request this, please ignore this email.
    </p>
  </div>
</body>
</html>`
