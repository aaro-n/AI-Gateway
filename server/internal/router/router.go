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
		if providerInfo == nil || !providerInfo.Enabled {
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

// RouteAllDirect 返回所有候选项（按 weight 排序，跳过熔断/cooling 中的项）
func (r *ModelRouter) RouteAllDirect(modelID string, keyID uint) ([]*RouteResult, error) {
	r.cooldownManager.ClearExpiredCooldowns()

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

	var results []*RouteResult
	for _, pm := range pms {
		if pm.Provider == nil || !pm.Provider.Enabled {
			continue
		}
		pmCopy := pm
		result := &RouteResult{
			Provider:      pm.Provider,
			ProviderModel: &pmCopy,
		}
		if !r.cooldownManager.IsCooldown(pm.Provider.ID, pm.ID) {
			results = append(results, result)
		}
	}
	return results, nil
}

// RouteAll 返回所有启用的候选项（按 weight DESC 排序，跳过熔断/cooling 中的项）
func (r *ModelRouter) RouteAll(name string) ([]*RouteResult, error) {
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

	var results []*RouteResult
	for _, mapping := range mappings {
		if mapping.Provider == nil || !mapping.Provider.Enabled {
			continue
		}
		if mapping.ProviderModel == nil || !mapping.ProviderModel.IsAvailable {
			continue
		}

		result := &RouteResult{
			Provider:      mapping.Provider,
			ProviderModel: mapping.ProviderModel,
		}
		if !r.cooldownManager.IsCooldown(mapping.Provider.ID, mapping.ProviderModel.ID) {
			results = append(results, result)
		}
	}
	return results, nil
}

func (r *ModelRouter) RecordRateLimit(providerID uint, providerModelID uint) {
	r.cooldownManager.Record429(providerID, providerModelID)
}

// RecordError 记录通用上游错误（5xx, 超时等），触发熔断计数。
func (r *ModelRouter) RecordError(providerID uint, providerModelID uint) {
	r.cooldownManager.RecordError(providerID, providerModelID)
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
