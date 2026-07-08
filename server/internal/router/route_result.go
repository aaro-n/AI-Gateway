package router

import (
	"ai-gateway/internal/model"
)

// RouteResult 路由结果。
// 上游协议的执行由 UnifiedGatewayHandler 通过 registry 动态创建。
type RouteResult struct {
	Provider      *model.Provider
	ProviderModel *model.ProviderModel
}

// SupportProtocol 检查是否支持指定协议
func (r *RouteResult) SupportProtocol(protocol string) bool {
	return r.Provider.EndpointFor(protocol) != ""
}

// GetProviderProtocols 获取所有支持的协议列表
func (r *RouteResult) GetProviderProtocols() []string {
	return r.Provider.SupportedProtocols()
}
