package model

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex"`
	PasswordHash string
	Role         string `gorm:"default:admin"`
	Enabled      bool   `gorm:"type:boolean;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt
}

func (User) TableName() string {
	return "users"
}

type Provider struct {
	ID               uint   `gorm:"primaryKey"`
	Name             string `gorm:"uniqueIndex"`
	OpenAIBaseURL    string `gorm:"column:openai_base_url"`
	AnthropicBaseURL string `gorm:"column:anthropic_base_url"`
	GeminiBaseURL    string `gorm:"column:gemini_base_url"`
	APIKey           string
	Enabled          bool   `gorm:"type:boolean;default:true"`
	Priority         int    `gorm:"default:0"`
	Config           string `gorm:"type:text"`
	Endpoints        string `gorm:"type:text"` // JSON: {"openai":"https://...","gemini":"https://..."}
	LastSyncAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	Models           []ProviderModel
}

func (Provider) TableName() string {
	return "providers"
}

type ProviderModel struct {
	ID             uint   `gorm:"primaryKey"`
	ProviderID     uint   `gorm:"uniqueIndex:idx_provider_model"`
	ModelID        string `gorm:"uniqueIndex:idx_provider_model"`
	DisplayName    string
	OwnedBy        string
	ContextWindow  int     `gorm:"default:0"`
	MaxOutput      int     `gorm:"default:0"`
	InputPrice     float64 `gorm:"default:0"`
	OutputPrice    float64 `gorm:"default:0"`
	SupportsVision bool    `gorm:"type:boolean;default:false"`
	SupportsTools  bool    `gorm:"type:boolean;default:true"`
	SupportsStream bool    `gorm:"type:boolean;default:true"`
	Metadata       string  `gorm:"type:text"`
	IsAvailable    bool    `gorm:"type:boolean;default:true"`
	Source         string  `gorm:"default:sync"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Provider       *Provider `gorm:"foreignKey:ProviderID;references:ID"`
}

func (ProviderModel) TableName() string {
	return "provider_models"
}

type Model struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex;column:name"`
	Enabled   bool   `gorm:"type:boolean;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	Mappings  []ModelMapping `gorm:"foreignKey:ModelID"`
}

func (Model) TableName() string {
	return "models"
}

type ModelMapping struct {
	ID              uint `gorm:"primaryKey"`
	ModelID         uint `gorm:"index"`
	ProviderID      uint `gorm:"index"`
	ProviderModelID uint `gorm:"index"`
	Weight          int  `gorm:"default:1"`
	Enabled         bool `gorm:"type:boolean;default:true"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Provider        *Provider      `gorm:"foreignKey:ProviderID;references:ID"`
	ProviderModel   *ProviderModel `gorm:"foreignKey:ProviderModelID;references:ID"`
}

func (ModelMapping) TableName() string {
	return "model_mappings"
}

type MCP struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"uniqueIndex;size:200;not null"`
	Type string `gorm:"type:varchar(20);not null"`

	Target string `gorm:"type:text"`
	Params string `gorm:"type:text"`

	Enabled      bool   `gorm:"default:true"`
	Capabilities string `gorm:"type:text"`
	LastSyncAt   *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (MCP) TableName() string {
	return "mcps"
}

type MCPTool struct {
	ID          uint   `gorm:"primaryKey"`
	MCPID       uint   `gorm:"index;not null"`
	Name        string `gorm:"index;size:200;not null"`
	Description string `gorm:"type:text"`
	InputSchema string `gorm:"type:text"`
	Enabled     bool   `gorm:"default:true"`

	CreatedAt time.Time
	UpdatedAt time.Time

	MCP *MCP `gorm:"foreignKey:MCPID"`
}

func (MCPTool) TableName() string {
	return "mcp_tools"
}

type MCPResource struct {
	ID          uint   `gorm:"primaryKey"`
	MCPID       uint   `gorm:"index;not null"`
	Name        string `gorm:"size:200"`
	Description string `gorm:"type:text"`
	URI         string `gorm:"type:text;not null"`
	MimeType    string `gorm:"size:100"`
	Enabled     bool   `gorm:"default:true"`

	CreatedAt time.Time
	UpdatedAt time.Time

	MCP *MCP `gorm:"foreignKey:MCPID"`
}

func (MCPResource) TableName() string {
	return "mcp_resources"
}

type MCPPrompt struct {
	ID          uint   `gorm:"primaryKey"`
	MCPID       uint   `gorm:"index;not null"`
	Name        string `gorm:"index;size:200;not null"`
	Description string `gorm:"type:text"`
	Arguments   string `gorm:"type:text"`
	Enabled     bool   `gorm:"default:true"`

	CreatedAt time.Time
	UpdatedAt time.Time

	MCP *MCP `gorm:"foreignKey:MCPID"`
}

func (MCPPrompt) TableName() string {
	return "mcp_prompts"
}

type Key struct {
	ID         uint   `gorm:"primaryKey"`
	Key        string `gorm:"uniqueIndex"`
	Name       string
	Enabled    bool   `gorm:"type:boolean;default:true"`
	AccessMode string `gorm:"type:varchar(50);default:'hybrid'"` // "mapping", "direct", "hybrid"

	ExpiresAt *time.Time
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt

	Models       []KeyModel
	Providers    []KeyProvider
	MCPTools     []KeyMCPTool
	MCPResources []KeyMCPResource
	MCPPrompts   []KeyMCPPrompt
	Formats      []KeyFormat `gorm:"foreignKey:KeyID"`
}

func (Key) TableName() string {
	return "keys"
}

type KeyFormat struct {
	ID           uint   `gorm:"primaryKey"`
	KeyID        uint   `gorm:"index;not null"`
	Format       string `gorm:"size:50;index;not null"`        // "openai", "anthropic", "gemini"
	FormattedKey string `gorm:"uniqueIndex;size:255;not null"` // "sk-...", "sk-ant-...", "AIzaSy..."
	CreatedAt    time.Time
}

func (KeyFormat) TableName() string {
	return "key_formats"
}

type KeyModel struct {
	ID      uint `gorm:"primaryKey"`
	KeyID   uint `gorm:"index"`
	ModelID uint `gorm:"index"`

	CreatedAt time.Time

	Model *Model `gorm:"foreignKey:ModelID"`
}

func (KeyModel) TableName() string {
	return "key_models"
}

type KeyProvider struct {
	ID         uint `gorm:"primaryKey"`
	KeyID      uint `gorm:"index;not null"`
	ProviderID uint `gorm:"index;not null"`

	CreatedAt time.Time
	Provider  *Provider `gorm:"foreignKey:ProviderID"`
}

func (KeyProvider) TableName() string {
	return "key_providers"
}

// KeyProviderModel API 密钥的"模型厂商直通"白名单（模型ID级别）。
// 与 KeyProvider（厂商级别）不同，这里精确到具体的 ProviderModel，
// 使得"直通模型ID"与"映射模型ID"可以在同一维度比较，避免重复。
type KeyProviderModel struct {
	ID              uint `gorm:"primaryKey"`
	KeyID           uint `gorm:"index;not null;uniqueIndex:idx_key_provider_model"`
	ProviderModelID uint `gorm:"index;not null;uniqueIndex:idx_key_provider_model"`

	CreatedAt time.Time

	ProviderModel *ProviderModel `gorm:"foreignKey:ProviderModelID"`
}

func (KeyProviderModel) TableName() string {
	return "key_provider_models"
}

type KeyMCPTool struct {
	ID     uint `gorm:"primaryKey"`
	KeyID  uint `gorm:"index;not null"`
	ToolID uint `gorm:"index;not null"`

	CreatedAt time.Time

	Tool *MCPTool `gorm:"foreignKey:ToolID"`
}

func (KeyMCPTool) TableName() string {
	return "key_mcp_tools"
}

type KeyMCPResource struct {
	ID         uint `gorm:"primaryKey"`
	KeyID      uint `gorm:"index;not null"`
	ResourceID uint `gorm:"index;not null"`

	CreatedAt time.Time

	Resource *MCPResource `gorm:"foreignKey:ResourceID"`
}

func (KeyMCPResource) TableName() string {
	return "key_mcp_resources"
}

type KeyMCPPrompt struct {
	ID       uint `gorm:"primaryKey"`
	KeyID    uint `gorm:"index;not null"`
	PromptID uint `gorm:"index;not null"`

	CreatedAt time.Time

	Prompt *MCPPrompt `gorm:"foreignKey:PromptID"`
}

func (KeyMCPPrompt) TableName() string {
	return "key_mcp_prompts"
}

type ModelLog struct {
	ID              uint `gorm:"primaryKey"`
	Source          string
	ClientIPs       string `gorm:"column:client_ips"`
	KeyID           uint   `gorm:"index"`
	KeyName         string
	Model           string
	ProviderID      uint `gorm:"index"`
	ProviderName    string
	ActualModelID   string `gorm:"index"`
	ActualModelName string
	CallMethod      string
	CachedTokens    int `gorm:"default:0"`
	InputTokens     int `gorm:"default:0"`
	OutputTokens    int `gorm:"default:0"`
	TotalTokens     int `gorm:"default:0"`
	LatencyMs       int `gorm:"default:0"`
	Status          string
	ErrorMsg        string    `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"index"`
}

func (ModelLog) TableName() string {
	return "model_logs"
}

func (u *ModelLog) String() string {
	return fmt.Sprintf("[%s - %s] %s calling model %s, provider:(%s/%s), method:%s, tokens:(C:%d/I:%d/O:%d/T:%d), time:%dms, status:%s",
		u.Source, u.ClientIPs, u.KeyName, u.Model,
		u.ProviderName, u.ActualModelName, u.CallMethod,
		u.CachedTokens, u.InputTokens, u.OutputTokens, u.TotalTokens,
		u.LatencyMs, u.Status,
	)
}

type TestResult struct {
	ID              uint   `gorm:"primaryKey"`
	ProviderID      uint   `gorm:"index"`
	ProviderModelID uint   `gorm:"index;default:0"`
	ModelID         string `gorm:"index"`
	Protocol        string `gorm:"index"`
	Success         bool
	CallMethod      string
	LatencyMs       int64
	InputTokens     int
	OutputTokens    int
	Response        string    `gorm:"type:text"`
	Error           string    `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"index"`
}

func (TestResult) TableName() string {
	return "test_results"
}

type MCPLog struct {
	ID         uint      `gorm:"primaryKey"`
	Source     string    `gorm:"column:source"`
	ClientIPs  string    `gorm:"column:client_ips"`
	KeyID      uint      `gorm:"index"`
	KeyName    string    `gorm:"column:key_name"`
	MCPID      uint      `gorm:"index;column:mcp_id"`
	MCPName    string    `gorm:"column:mcp_name"`
	MCPType    string    `gorm:"column:mcp_type"`
	CallType   string    `gorm:"index;column:call_type"`
	CallMethod string    `gorm:"column:call_method"`
	CallTarget string    `gorm:"column:call_target"`
	InputSize  int       `gorm:"default:0;column:input_size"`
	OutputSize int       `gorm:"default:0;column:output_size"`
	LatencyMs  int       `gorm:"default:0;column:latency_ms"`
	Status     string    `gorm:"column:status"`
	ErrorMsg   string    `gorm:"type:text;column:error_msg"`
	CreatedAt  time.Time `gorm:"index;column:created_at"`
}

func (MCPLog) TableName() string {
	return "mcp_logs"
}

func (u *MCPLog) String() string {
	return fmt.Sprintf("[%s - %s] %s calling MCP %s/%s, type:%s/%s, size:(I:%d/O:%d), time:%dms, status:%s",
		u.Source, u.ClientIPs, u.KeyName,
		u.MCPName, u.CallTarget, u.MCPType, u.CallType,
		u.InputSize, u.OutputSize,
		u.LatencyMs, u.Status,
	)
}

func InitDB(
	dbType, dbPath, dbHost string, dbPort int, dbUser, dbPassword, dbName string,
	maxOpen, maxIdle int, maxLifetime, maxIdleTime time.Duration,
	debug bool,
) error {
	var dialector gorm.Dialector
	var err error

	switch dbType {
	case "postgres":
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
			dbHost, dbPort, dbUser, dbPassword, dbName)
		log.Printf("[Database] Connecting to PostgreSQL: host=%s, port=%d, dbname=%s", dbHost, dbPort, dbName)
		dialector = postgres.Open(dsn)
	case "sqlite":
		dir := filepath.Dir(dbPath)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		dsn := dbPath + "?_loc=auto"
		log.Printf("[Database] Connecting to SQLite: path=%s", dbPath)
		dialector = sqlite.Open(dsn)
	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	logLevel := logger.Silent
	if debug {
		logLevel = logger.Info
	}

	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		log.Printf("[Database] Failed to connect to database: %v", err)
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("[Database] Failed to get underlying sql.DB: %v", err)
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	if dbType == "sqlite" {
		maxOpen = 1
		maxIdle = 1
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(maxLifetime)
	sqlDB.SetConnMaxIdleTime(maxIdleTime)
	log.Printf("[Database] Connection pool configured: MaxOpen=%d, MaxIdle=%d, MaxLifetime=%v, MaxIdleTime=%v", maxOpen, maxIdle, maxLifetime, maxIdleTime)

	log.Printf("[Database] Database connection successful")

	if err := autoMigrate(); err != nil {
		return err
	}

	if err := migrateKeyFormats(); err != nil {
		return err
	}

	return nil
}

func autoMigrate() error {
	return DB.AutoMigrate(
		&User{},
		&Provider{},
		&ProviderModel{},
		&Model{},
		&ModelMapping{},
		&MCP{},
		&MCPTool{},
		&MCPResource{},
		&MCPPrompt{},
		&Key{},
		&KeyFormat{},
		&KeyModel{},
		&KeyProvider{},
		&KeyProviderModel{},
		&KeyMCPTool{},
		&KeyMCPResource{},
		&KeyMCPPrompt{},
		&ModelLog{},
		&TestResult{},
		&MCPLog{},
	)
}

func migrateKeyFormats() error {
	var keys []Key
	if err := DB.Find(&keys).Error; err != nil {
		return err
	}
	for _, k := range keys {
		var count int64
		DB.Model(&KeyFormat{}).Where("key_id = ?", k.ID).Count(&count)
		if count == 0 && k.Key != "" {
			kf := KeyFormat{
				KeyID:        k.ID,
				Format:       "openai",
				FormattedKey: k.Key,
			}
			if err := DB.Create(&kf).Error; err != nil {
				log.Printf("[Database] Failed to migrate key format for key ID %d: %v", k.ID, err)
				return err
			}
			log.Printf("[Database] Migrated key ID %d Key column to openai format", k.ID)
		}
	}
	return nil
}

func GetDB() *gorm.DB {
	return DB
}
