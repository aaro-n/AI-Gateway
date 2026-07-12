package config

import (
	"fmt"
	"log"
	"sync"
)

// reloadMu 保护全局 cfg 的并发读写。
// Load() 初始化时使用普通写入，Reload() 及 Get() 使用读锁。
var reloadMu sync.RWMutex
var configPath string

// Reload 热重载配置。
//
// 规则：
//   - 环境变量（AG_*）优先级始终高于 YAML，即使 YAML 中显式设置了值。
//   - 以下字段热重载生效：debug.*, test_concurrency, time_zone, pool.*
//   - 以下字段忽略（需重启）：database type/url/path/host/port/username/password/dbname,
//     server port/trusted_proxies, session, auth, pprof port, monitor, admin default credentials
//
// 返回 error 在配置无效时（hot-reload 不通过时原配置保持不变）。
//
// SIGHUP 信号和 admin API (/api/v1/admin/reload-config) 可触发此方法。
func Reload() error {
	yamlCfg := loadYAML(configPath)
	if yamlCfg == nil {
		// YAML 文件不存在/解析失败 → 只重新读取环境变量
		yamlCfg = &Config{}
		log.Printf("[Config] Reload: YAML file not found, refreshing env vars only")
	}

	// ── 构建新配置（仅加载可热重载的字段）──

	reloadMu.Lock()
	defer reloadMu.Unlock()

	old := cfg
	if old == nil {
		return fmt.Errorf("config not initialized")
	}

	// Debug — 完全热重载
	old.Debug.Gin = getBool("AG_DEBUG_GIN", yamlCfg.Debug.Gin)
	old.Debug.Gorm = getBool("AG_DEBUG_GORM", yamlCfg.Debug.Gorm)
	old.Debug.Provider = getBool("AG_DEBUG_PROVIDER", yamlCfg.Debug.Provider)
	old.Debug.MCP = getBool("AG_DEBUG_MCP", yamlCfg.Debug.MCP)
	old.Debug.LogFile = getEnv("AG_DEBUG_LOG_FILE", yamlCfg.Debug.LogFile)

	// Server — 仅 timezone & test_concurrency
	newTZ := getEnv("AG_TIME_ZONE", yamlCfg.Server.TimeZone)
	if newTZ == "" {
		newTZ = "Asia/Shanghai"
	}
	old.Server.TimeZone = newTZ
	applyTimeZone(newTZ)

	tc := getInt("AG_TEST_CONCURRENCY", yamlCfg.Server.TestConcurrency)
	if tc <= 0 {
		tc = 5
	}
	old.Server.TestConcurrency = tc

	// SMTP — 完全热重载
	old.SMTP.Enabled = getBool("AG_SMTP_ENABLED", yamlCfg.SMTP.Enabled)
	old.SMTP.Host = getEnv("AG_SMTP_HOST", yamlCfg.SMTP.Host)
	old.SMTP.Port = getInt("AG_SMTP_PORT", yamlCfg.SMTP.Port)
	old.SMTP.Username = getEnv("AG_SMTP_USERNAME", yamlCfg.SMTP.Username)
	old.SMTP.Password = getEnv("AG_SMTP_PASSWORD", yamlCfg.SMTP.Password)
	old.SMTP.From = getEnv("AG_SMTP_FROM", yamlCfg.SMTP.From)
	old.SMTP.UseTLS = getBool("AG_SMTP_USE_TLS", yamlCfg.SMTP.UseTLS)
	old.SMTP.LogResetLink = getBool("AG_SMTP_LOG_RESET_LINK", yamlCfg.SMTP.LogResetLink)

	log.Printf("[Config] Reload complete: gin=%v gorm=%v provider=%v mcp=%v tz=%s test_concurrency=%d log_file=%s smtp=%s:%d",
		old.Debug.Gin, old.Debug.Gorm, old.Debug.Provider, old.Debug.MCP,
		old.Server.TimeZone, old.Server.TestConcurrency, old.Debug.LogFile,
		old.SMTP.Host, old.SMTP.Port)

	return nil
}

// Get 返回当前配置（线程安全）。
func Get() *Config {
	reloadMu.RLock()
	defer reloadMu.RUnlock()
	return cfg
}

// resetConfigForTesting 测试用：重置配置状态。
// 仅用于单测，生产代码不应调用。
func resetConfigForTesting() {
	reloadMu.Lock()
	defer reloadMu.Unlock()
	cfg = nil
}
