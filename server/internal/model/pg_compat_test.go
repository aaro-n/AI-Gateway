package model

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// pgTestDSN 从环境变量 AG_TEST_POSTGRES_URL 读取 PostgreSQL 连接串。
// 未设置时跳过测试。用法：
//
//	docker compose up -d postgres
//	AG_TEST_POSTGRES_URL="postgres://postgres:postgres@localhost:5432/ai_gateway_test?sslmode=disable" go test ./internal/model/ -run PG -v
//
// CI 中可配合 docker-compose.yml 的 postgres 服务使用。
func pgTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("AG_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("AG_TEST_POSTGRES_URL not set, skipping PostgreSQL integration test")
	}
	return dsn
}

// newPGDB 打开一个独立 PG 连接（不复用全局 DB），避免污染其他测试。
func newPGDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := pgTestDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestPG_AutoMigrate 验证所有模型能在 PostgreSQL 上完成 auto-migrate。
// 这是 PG 兼容性的基础检查：任何 GORM 标签不兼容都会在此失败。
func TestPG_AutoMigrate(t *testing.T) {
	db := newPGDB(t)

	// 清理可能残留的表（按依赖逆序）
	tables := []string{
		"mcp_logs", "model_logs", "test_results",
		"key_mcp_prompts", "key_mcp_resources", "key_mcp_tools",
		"key_provider_models", "key_providers", "key_models", "key_formats",
		"keys", "mcp_prompts", "mcp_resources", "mcp_tools", "mcps",
		"model_mappings", "models", "provider_models", "providers", "users",
	}
	for _, tbl := range tables {
		db.Migrator().DropTable(tbl)
	}

	if err := db.AutoMigrate(
		&User{}, &Provider{}, &ProviderModel{}, &Model{}, &ModelMapping{},
		&MCP{}, &MCPTool{}, &MCPResource{}, &MCPPrompt{},
		&Key{}, &KeyFormat{}, &KeyModel{}, &KeyProvider{}, &KeyProviderModel{},
		&KeyMCPTool{}, &KeyMCPResource{}, &KeyMCPPrompt{},
		&ModelLog{}, &TestResult{}, &MCPLog{},
	); err != nil {
		t.Fatalf("PostgreSQL AutoMigrate failed: %v", err)
	}
}

// TestPG_BooleanColumn 验证 boolean 列在 PostgreSQL 上用 true/false 字面量赋值。
// 这是 db.go:585-586 修复的回归测试——之前用 `enabled = 1` 在 PG 上会报错。
func TestPG_BooleanColumn(t *testing.T) {
	db := newPGDB(t)
	db.Migrator().DropTable("key_models")
	if err := db.AutoMigrate(&KeyModel{}); err != nil {
		t.Fatalf("AutoMigrate KeyModel failed: %v", err)
	}

	// 插入一条 enabled=false 的记录
	km := KeyModel{KeyID: 99991, ModelID: 99991, Enabled: false}
	if err := db.Create(&km).Error; err != nil {
		t.Fatalf("create KeyModel failed: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&km) })

	// 用 true/false 字面量更新（与 db.go:585 修复后的语句一致）
	if err := db.Exec("UPDATE key_models SET enabled = true WHERE enabled IS NULL OR enabled = false").Error; err != nil {
		t.Fatalf("boolean update with true/false literal failed on PostgreSQL: %v", err)
	}

	// 验证更新生效
	var got KeyModel
	if err := db.First(&got, km.ID).Error; err != nil {
		t.Fatalf("query KeyModel failed: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true after update, got false")
	}
}

// TestPG_TimestampRoundTrip 验证时间戳在 PostgreSQL 上的存储与读取往返一致，
// 并验证按用户时区（time.Local）分组日期归属正确。
func TestPG_TimestampRoundTrip(t *testing.T) {
	db := newPGDB(t)
	db.Migrator().DropTable("model_logs")
	if err := db.AutoMigrate(&ModelLog{}); err != nil {
		t.Fatalf("AutoMigrate ModelLog failed: %v", err)
	}

	// 用固定时区构造时间，避免依赖当前 time.Local
	loc, _ := time.LoadLocation("Asia/Shanghai")
	ts := time.Date(2026, 7, 12, 2, 30, 0, 0, loc) // 2026-07-12 02:30 +08:00 = 2026-07-11 18:30 UTC

	ml := ModelLog{
		Model:       "test-model",
		Status:      "success",
		TotalTokens: 100,
		CreatedAt:   ts,
	}
	if err := db.Create(&ml).Error; err != nil {
		t.Fatalf("create ModelLog failed: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&ml) })

	// 读取回来，验证时间值一致（GORM 应保留时区信息）
	var got ModelLog
	if err := db.First(&got, ml.ID).Error; err != nil {
		t.Fatalf("query ModelLog failed: %v", err)
	}

	// 比较绝对时刻（UTC 等价），忽略时区表示差异
	if !got.CreatedAt.UTC().Equal(ts.UTC()) {
		t.Fatalf("timestamp round-trip mismatch: got %v, want %v", got.CreatedAt, ts)
	}

	// 验证按 Asia/Shanghai 分组应归属 2026-07-12（本地日期）
	if got.CreatedAt.In(loc).Format("2006-01-02") != "2026-07-12" {
		t.Fatalf("expected local date 2026-07-12, got %s", got.CreatedAt.In(loc).Format("2006-01-02"))
	}
	// 验证按 UTC 分组应归属 2026-07-11
	if got.CreatedAt.UTC().Format("2006-01-02") != "2026-07-11" {
		t.Fatalf("expected UTC date 2026-07-11, got %s", got.CreatedAt.UTC().Format("2006-01-02"))
	}
}
