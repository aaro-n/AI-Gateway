/**
 * 协议前端定义 — 与后端 protocols/xxx/ 一一对应
 * 
 * 架构：选择什么协议，就执行对应文件夹下的代码。
 * 后端：protocols/openai/ → provider.go, request_parse.go, ...
 * 前端：protocols/openai.ts → 表单、显示、密钥格式、预设模型...
 */

/** 协议名 → 配置的完整映射 */
export interface ProtocolFrontendConfig {
  name: string
  label: string
  tagType: '' | 'success' | 'primary' | 'warning' | 'danger' | 'info'
  keyPrefix: string
  defaultBaseURL: string
  description: string
  /** 新建供应商时需要的额外表单字段（除 base_url 外） */
  extraFormFields: ProtocolFormField[]
  /** 预设模型（快捷添加） */
  presetModels: string[]
  /** 该协议不支持的功能列表（前端据此禁用控件） */
  unsupportedFeatures: string[]
  /** 特殊行为 */
  behaviors: ProtocolBehaviors
}

export interface ProtocolFormField {
  key: string
  label: string
  type: 'text' | 'url' | 'number' | 'switch'
  placeholder?: string
  required?: boolean
}

export interface ProtocolBehaviors {
  /** 模型 ID 是否可编辑 */
  modelIdEditable: boolean
  /** 上下文窗口是否可编辑 */
  contextWindowEditable: boolean
  /** 价格是否可编辑 */
  priceEditable: boolean
  /** 同步按钮文案（为空则用默认"获取厂商模型列表"） */
  syncButtonLabel?: string
}
