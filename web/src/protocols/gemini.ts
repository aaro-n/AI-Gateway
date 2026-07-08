import type { ProtocolFrontendConfig } from './types'

export const geminiConfig: ProtocolFrontendConfig = {
  name: 'gemini', label: 'Google Gemini', tagType: 'warning', keyPrefix: 'AIza',
  defaultBaseURL: 'https://generativelanguage.googleapis.com/v1beta',
  description: 'Google Gemini API 协议',
  extraFormFields: [
    { key: 'region', label: 'Region', type: 'text', placeholder: 'us-central1（可选）' },
  ],
  presetModels: ['gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-1.5-flash', 'gemini-1.5-pro'],
  unsupportedFeatures: ['frequency_penalty', 'presence_penalty', 'seed', 'reasoning_effort'],
  behaviors: { modelIdEditable: true, contextWindowEditable: true, priceEditable: true },
}
