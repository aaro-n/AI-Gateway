import type { ProtocolFrontendConfig } from './types'

export const openaiConfig: ProtocolFrontendConfig = {
  name: 'openai', label: 'OpenAI', tagType: 'success', keyPrefix: 'sk-',
  defaultBaseURL: 'https://api.openai.com/v1',
  description: 'OpenAI API 兼容协议',
  extraFormFields: [],
  presetModels: ['gpt-4o', 'gpt-4o-mini', 'gpt-4', 'o1-mini', 'o3-mini'],
  unsupportedFeatures: [],
  behaviors: { modelIdEditable: true, contextWindowEditable: true, priceEditable: true },
}
