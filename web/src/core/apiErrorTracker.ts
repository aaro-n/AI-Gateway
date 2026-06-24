// API 错误追踪 composable
// 用于统计和上报前端 API 调用错误

import { ref } from 'vue'

interface ApiError {
  url: string
  method: string
  status: number
  message: string
  time: Date
}

const apiErrors = ref<ApiError[]>([])
const maxErrors = 50

/**
 * 记录一个 API 错误
 */
export function trackApiError(url: string, method: string, status: number, message: string) {
  apiErrors.value.unshift({ url, method, status, message, time: new Date() })

  // 保持最大数量
  if (apiErrors.value.length > maxErrors) {
    apiErrors.value = apiErrors.value.slice(0, maxErrors)
  }

  // 在控制台输出便于调试
  if (status >= 500) {
    console.error(`[API Error] ${method} ${url} → ${status}: ${message}`)
  } else {
    console.warn(`[API Warn] ${method} ${url} → ${status}: ${message}`)
  }
}

/**
 * 获取所有已追踪的 API 错误
 */
export function getApiErrors() {
  return apiErrors.value
}

/**
 * 清空错误列表
 */
export function clearApiErrors() {
  apiErrors.value = []
}

/**
 * 错误统计
 */
export function getApiErrorStats() {
  const byStatus: Record<number, number> = {}
  let total5xx = 0
  let total4xx = 0

  for (const e of apiErrors.value) {
    byStatus[e.status] = (byStatus[e.status] || 0) + 1
    if (e.status >= 500) total5xx++
    else if (e.status >= 400) total4xx++
  }

  return {
    total: apiErrors.value.length,
    total5xx,
    total4xx,
    byStatus,
    latestError: apiErrors.value[0] || null,
  }
}
