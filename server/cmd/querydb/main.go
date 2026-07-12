package main
package main

import (
	"fmt"
	"ai-gateway/internal/model"
)

func main() {
	// Initialize DB from config
	if err := model.InitDB(); err != nil {
		fmt.Println("DB init error:", err)
		return
	}

	// Query providers
	var providers []model.Provider
	if err := model.DB.Find(&providers).Error; err != nil {
		fmt.Println("Query error:", err)
		return
	}
	fmt.Println("=== Providers ===")
	for _, p := range providers {
		fmt.Printf("ID=%d Name=%s Protocol=%s Enabled=%v CreatedAt=%s\n",
			p.ID, p.Name, p.Protocol, p.Enabled, p.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// Query provider models
	var pms []model.ProviderModel
	if err := model.DB.Find(&pms).Error; err != nil {
		fmt.Println("Query error:", err)
		return
	}
	fmt.Println("\n=== Provider Models ===")
	for _, pm := range pms {
		fmt.Printf("ID=%d ProviderID=%d ModelName=%s Enabled=%v\n",
			pm.ID, pm.ProviderID, pm.ModelName, pm.Enabled)
	}

	// Query models
	var models []model.Model
	if err := model.DB.Find(&models).Error; err != nil {
		fmt.Println("Query error:", err)
		return
	}
	fmt.Println("\n=== Models ===")
	for _, m := range models {
		fmt.Printf("ID=%d Name=%s ProviderModelID=%d DefaultTokenLimit=%d\n",
			m.ID, m.Name, m.ProviderModelID, m.DefaultTokenLimit)
	}
}
