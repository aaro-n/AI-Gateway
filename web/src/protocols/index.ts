/**
 * 协议前端注册表 — 与后端 core/registry/registry.go 对应
 *
 * 新增协议只需在此加一行 import + 一行注册。
 * 所有 Vue 组件从本文件获取协议配置，不再硬编码。
 */
import type { ProtocolFrontendConfig } from './types'
import { openaiConfig } from './openai'
import { anthropicConfig } from './anthropic'
import { geminiConfig } from './gemini'
import { deepseekConfig } from './deepseek'
import { openrouterConfig } from './openrouter'

/** 所有已注册协议 */
const ALL: ProtocolFrontendConfig[] = [
  openaiConfig,
  anthropicConfig,
  geminiConfig,
  deepseekConfig,
  openrouterConfig,
]

/** 协议名 → 配置的快速查找表 */
const MAP: Record<string, ProtocolFrontendConfig> = {}
ALL.forEach(c => { MAP[c.name] = c })

/** 根据协议名获取配置 */
export function getProtocolConfig(name: string): ProtocolFrontendConfig | undefined {
  return MAP[name]
}

/** 获取所有已注册协议 */
export function getAllProtocols(): ProtocolFrontendConfig[] {
  return ALL
}

/** 根据协议名获取人类可读标签 */
export function getProtocolLabel(name: string): string {
  return MAP[name]?.label || name.toUpperCase()
}

/** 根据协议名获取 Element Plus tag type */
export function getProtocolTagType(name: string): string {
  return MAP[name]?.tagType || ''
}

/** 获取所有已知协议名 */
export function getKnownProtocolNames(): string[] {
  return ALL.map(p => p.name)
}

/** 获取协议的预设模型列表 */
export function getPresetModels(name: string): string[] {
  return MAP[name]?.presetModels || []
}

/** 获取协议的特殊行为配置 */
export function getBehaviors(name: string) {
  return MAP[name]?.behaviors || { modelIdEditable: true, contextWindowEditable: true, priceEditable: true }
}

/** 从 provider 对象中提取所有有值的端点（通用工具函数） */
export function getProviderEndpoints(provider: any): { name: string; url: string }[] {
  if (!provider) return []
  if (provider.endpoints && typeof provider.endpoints === 'object') {
    return Object.entries(provider.endpoints)
      .filter(([, v]) => v)
      .map(([k]) => ({ name: k, url: '' }))
  }
  // fallback: 扁平字段
  return getKnownProtocolNames()
    .map(name => ({ name, url: provider[name + '_base_url'] || '' }))
    .filter(ep => ep.url)
}
