import type { ProtocolFrontendConfig } from './types'

export const anthropicConfig: ProtocolFrontendConfig = {
  name: 'anthropic', label: 'Anthropic', tagType: 'primary', keyPrefix: 'sk-ant-',
  defaultBaseURL: 'https://api.anthropic.com',
  description: 'Anthropic Claude API 协议',
  extraFormFields: [],
  presetModels: ['claude-sonnet-4-20250514', 'claude-3-5-sonnet-latest', 'claude-3-5-haiku-latest'],
  unsupportedFeatures: ['frequency_penalty', 'presence_penalty', 'seed'],
  behaviors: { modelIdEditable: true, contextWindowEditable: true, priceEditable: true },
}
