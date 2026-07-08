import type { ProtocolFrontendConfig } from './types'

export const deepseekConfig: ProtocolFrontendConfig = {
  name: 'deepseek', label: 'DeepSeek', tagType: 'danger', keyPrefix: 'sk-',
  defaultBaseURL: 'https://api.deepseek.com/v1',
  description: 'DeepSeek API 协议（OpenAI 兼容）',
  extraFormFields: [],
  presetModels: ['deepseek-chat', 'deepseek-reasoner'],
  unsupportedFeatures: ['top_k'],
  behaviors: { modelIdEditable: true, contextWindowEditable: true, priceEditable: true },
}
