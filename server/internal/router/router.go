package router

import (
	"ai-gateway/internal/model"
)

var globalRouter = &ModelRouter{
	cooldownManager: NewCooldownManager(),
}

func GetRouter() *ModelRouter {
	return globalRouter
}

type ModelRouter struct {
	cooldownManager *CooldownManager
}

func (r *ModelRouter) Route(name string) (*RouteResult, error) {
	r.cooldownManager.ClearExpiredCooldowns()

	var m model.Model
	if err := model.DB.Where("name = ? AND enabled = ?", name, true).First(&m).Error; err != nil {
		return nil, nil
	}

	var mappings []model.ModelMapping
	if err := model.DB.Preload("Provider").Preload("ProviderModel").
		Where("model_id = ? AND enabled = ?", m.ID, true).
		Order("weight DESC").
		Find(&mappings).Error; err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, nil
	}

	var allProviders []RouteResult
	var availableProviders []RouteResult

	for _, mapping := range mappings {
		providerInfo := mapping.Provider
		if !providerInfo.Enabled {
			continue
		}

		if mapping.ProviderModel == nil || !mapping.ProviderModel.IsAvailable {
			continue
		}

		pm := mapping.ProviderModel

		result := RouteResult{
			Provider:      providerInfo,
			ProviderModel: pm,
		}
		allProviders = append(allProviders, result)

		if !r.cooldownManager.IsCooldown(providerInfo.ID, pm.ID) {
			availableProviders = append(availableProviders, result)
		}
	}

	if len(availableProviders) > 0 {
		return &availableProviders[0], nil
	}

	if len(allProviders) > 0 {
		earliest := r.cooldownManager.GetEarliestCooldownEnd(allProviders)
		if earliest != nil {
			return earliest, nil
		}
		return &allProviders[0], nil
	}

	return nil, nil
}

// RouteDirect 直通路由：按 provider_models.model_id 匹配。
// 如果 keyID > 0 且该 key 有 key_provider_models 白名单，则只允许白名单内的模型。
func (r *ModelRouter) RouteDirect(modelID string, keyID uint) (*RouteResult, error) {
	r.cooldownManager.ClearExpiredCooldowns()

	// 查询该 key 的直通白名单（enabled=true）
	var allowedPMIDs []uint
	if keyID > 0 {
		model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ? AND enabled = ?", keyID, true).Pluck("provider_model_id", &allowedPMIDs)
	}

	query := model.DB.Preload("Provider").
		Where("model_id = ? AND is_available = ?", modelID, true)
	if len(allowedPMIDs) > 0 {
		query = query.Where("id IN ?", allowedPMIDs)
	}

	var pms []model.ProviderModel
	if err := query.Find(&pms).Error; err != nil {
		return nil, err
	}

	if len(pms) == 0 {
		return nil, nil
	}

	var allProviders []RouteResult
	var availableProviders []RouteResult

	for _, pm := range pms {
		providerInfo := pm.Provider
		if providerInfo == nil || !providerInfo.Enabled {
			continue
		}

		pmCopy := pm
		result := RouteResult{
			Provider:      providerInfo,
			ProviderModel: &pmCopy,
		}
		allProviders = append(allProviders, result)

		if !r.cooldownManager.IsCooldown(providerInfo.ID, pm.ID) {
			availableProviders = append(availableProviders, result)
		}
	}

	if len(availableProviders) > 0 {
		return &availableProviders[0], nil
	}

	if len(allProviders) > 0 {
		return &allProviders[0], nil
	}

	return nil, nil
}

func (r *ModelRouter) RecordRateLimit(providerID uint, providerModelID uint) {
	r.cooldownManager.Record429(providerID, providerModelID)
}

func (r *ModelRouter) RecordSuccess(providerID uint, providerModelID uint) {
	r.cooldownManager.RecordSuccess(providerID, providerModelID)
}

func ClearCooldown(providerID uint, providerModelID uint) {
	globalRouter.cooldownManager.ClearCooldown(providerID, providerModelID)
}

func ClearAllCooldownsForProvider(providerID uint) {
	globalRouter.cooldownManager.ClearAllForProvider(providerID)
}
