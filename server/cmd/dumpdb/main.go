package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Provider struct {
	ID                uint   `gorm:"primaryKey"`
	Slug              string `gorm:"uniqueIndex;size:8"`
	Name              string
	OpenAIBaseURL     string `gorm:"column:openai_base_url"`
	AnthropicBaseURL  string `gorm:"column:anthropic_base_url"`
	GeminiBaseURL     string `gorm:"column:gemini_base_url"`
	DeepSeekBaseURL   string `gorm:"column:deepseek_base_url"`
	OpenRouterBaseURL string `gorm:"column:openrouter_base_url"`
	APIKey            string
	Enabled           bool
	Priority          int
	Endpoints         string `gorm:"type:text"`
	Config            string `gorm:"type:text"`
	CreatedAt         string
	UpdatedAt         string
}

type ProviderModel struct {
	ID             uint
	ProviderID     uint
	ModelID        string
	DisplayName    string
	OwnedBy        string
	ContextWindow  int
	MaxOutput      int
	InputPrice     float64
	OutputPrice    float64
	SupportsVision bool
	SupportsTools  bool
	SupportsStream bool
	IsAvailable    bool
	Source         string
}

type Key struct {
	ID         uint
	Name       string
	AccessMode string
	Enabled    bool
	ExpiresAt  *string
}

type KeyFormat struct {
	ID           uint
	KeyID        uint
	Format       string
	FormattedKey string
}

type KeyProviderModel struct {
	ID              uint
	KeyID           uint
	ProviderModelID uint
}

type ModelMapping struct {
	ID              uint
	KeyID           uint
	ModelName       string
	ProviderModelID uint
}

type Model struct {
	ID   uint
	Name string
}

func (Provider) TableName() string         { return "providers" }
func (ProviderModel) TableName() string    { return "provider_models" }
func (Key) TableName() string              { return "keys" }
func (KeyFormat) TableName() string        { return "key_formats" }
func (KeyProviderModel) TableName() string { return "key_provider_models" }
func (ModelMapping) TableName() string     { return "model_mappings" }
func (Model) TableName() string            { return "models" }

func main() {
	dsn := "host=localhost user=postgres password=postgres dbname=ai_gateway sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}

	out, _ := os.Create("/tmp/db_dump.txt")
	defer out.Close()

	// Providers
	var providers []Provider
	db.Find(&providers)
	fmt.Fprintln(out, "========== PROVIDERS ==========")
	for _, p := range providers {
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	// Provider Models
	var pms []ProviderModel
	db.Find(&pms)
	fmt.Fprintf(out, "\n========== PROVIDER_MODELS (%d) ==========\n", len(pms))
	for _, pm := range pms {
		b, _ := json.MarshalIndent(pm, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	// Keys
	var keys []Key
	db.Find(&keys)
	fmt.Fprintf(out, "\n========== KEYS (%d) ==========\n", len(keys))
	for _, k := range keys {
		b, _ := json.MarshalIndent(k, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	// Key Formats
	var kfs []KeyFormat
	db.Find(&kfs)
	fmt.Fprintf(out, "\n========== KEY_FORMATS (%d) ==========\n", len(kfs))
	for _, kf := range kfs {
		b, _ := json.MarshalIndent(kf, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	// Key Provider Models
	var kpms []KeyProviderModel
	db.Find(&kpms)
	fmt.Fprintf(out, "\n========== KEY_PROVIDER_MODELS (%d) ==========\n", len(kpms))
	for _, kpm := range kpms {
		b, _ := json.MarshalIndent(kpm, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	// Model Mappings
	var mms []ModelMapping
	db.Find(&mms)
	fmt.Fprintf(out, "\n========== MODEL_MAPPINGS (%d) ==========\n", len(mms))
	for _, mm := range mms {
		b, _ := json.MarshalIndent(mm, "", "  ")
		fmt.Fprintln(out, string(b))
	}

	fmt.Println("OK - written to /tmp/db_dump.txt")
}
