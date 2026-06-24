// OpenAI 协议前端定义
// 注：表单 Schema 由后端 GET /api/v1/protocols 动态提供，此文件仅作类型参考

export const openaiProtocol = {
  name: 'openai',
  label: 'OpenAI',
  keyPrefix: 'sk-',
  description: 'OpenAI API 兼容协议',
}

// 预设模型（快捷添加按钮）
export const presetModels = [
  'gpt-4o',
  'gpt-4o-mini',
  'gpt-4',
  'o1-mini',
  'o3-mini',
]
