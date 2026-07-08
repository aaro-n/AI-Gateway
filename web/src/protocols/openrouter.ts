import type { ProtocolFrontendConfig } from './types'

export const openrouterConfig: ProtocolFrontendConfig = {
  name: 'openrouter', label: 'OpenRouter', tagType: 'info', keyPrefix: 'sk-or-',
  defaultBaseURL: 'https://openrouter.ai/api/v1',
  description: 'OpenRouter API 协议（聚合多供应商）',
  extraFormFields: [],
  presetModels: [],
  unsupportedFeatures: [],
  behaviors: {
    modelIdEditable: false,
    contextWindowEditable: false,
    priceEditable: false,
    syncButtonLabel: '获取模型信息',
  },
}
