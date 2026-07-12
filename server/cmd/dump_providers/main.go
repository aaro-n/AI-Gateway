//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"ai-gateway/internal/model"
)

type OutProvider struct {
	ID        uint     `json:"id"`
	Name      string   `json:"name"`
	APIKey    string   `json:"api_key"`
	Endpoints string   `json:"endpoints"`
	Protocols []string `json:"protocols"`
	Models    []OutPM  `json:"models"`
}

type OutPM struct {
	ModelID        string `json:"model_id"`
	ContextWindow  int    `json:"context_window"`
	MaxOutput      int    `json:"max_output"`
	SupportsVision bool   `json:"supports_vision"`
	SupportsTools  bool   `json:"supports_tools"`
	SupportsStream bool   `json:"supports_stream"`
	OwnedBy        string `json:"owned_by"`
	Source         string `json:"source"`
}

func main() {
	if err := model.InitDB(); err != nil {
		fmt.Println("DB init error:", err)
		os.Exit(1)
	}

	var providers []model.Provider
	model.DB.Where("enabled = ?", true).Preload("Models").Find(&providers)

	result := make([]OutProvider, 0, len(providers))
	for _, p := range providers {
		op := OutProvider{
			ID:        p.ID,
			Name:      p.Name,
			APIKey:    p.APIKey,
			Endpoints: p.Endpoints,
			Protocols: p.SupportedProtocols(),
			Models:    make([]OutPM, 0, len(p.Models)),
		}
		for _, pm := range p.Models {
			op.Models = append(op.Models, OutPM{
				ModelID:        pm.ModelID,
				ContextWindow:  pm.ContextWindow,
				MaxOutput:      pm.MaxOutput,
				SupportsVision: pm.SupportsVision,
				SupportsTools:  pm.SupportsTools,
				SupportsStream: pm.SupportsStream,
				OwnedBy:        pm.OwnedBy,
				Source:         pm.Source,
			})
		}
		result = append(result, op)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}
