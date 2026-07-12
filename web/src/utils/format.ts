// 用户时区（IANA 时区名，如 "Asia/Shanghai"）。空值表示使用浏览器本地时区。
// 由 App.vue 在启动时从 user store 同步。
let _userTimeZone = ''

export function setUserTimeZone(tz: string) {
  _userTimeZone = tz
}

export function getUserTimeZone(): string {
  return _userTimeZone
}

// formatDateTime 将日期格式化为 "YYYY-MM-DD HH:mm:ss"。
// timeZone 为 IANA 时区名（如 "Asia/Shanghai"），空值用浏览器本地时区。
// 浏览器内置时区数据库自动处理夏令时。
export function formatDateTime(date: string | Date | null | undefined, timeZone?: string): string {
  if (!date) return '-'

  const d = typeof date === 'string' ? new Date(date) : date
  if (isNaN(d.getTime())) return '-'

  const tz = timeZone || _userTimeZone
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }
  if (tz) opts.timeZone = tz

  // toLocaleString 返回如 "2026/07/12 17:00:00" 或 "07/12/2026, 5:00:00 PM"
  // 用 formatToParts 精确提取各部分，保证格式一致
  const parts = new Intl.DateTimeFormat('en-CA', opts).formatToParts(d)
  const get = (type: string) => parts.find(p => p.type === type)?.value || ''
  const ms = String(d.getMilliseconds()).padStart(3, '0')

  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}.${ms}`
}

export function formatDate(date: string | Date | null | undefined, timeZone?: string): string {
  if (!date) return '-'

  const d = typeof date === 'string' ? new Date(date) : date
  if (isNaN(d.getTime())) return '-'

  const tz = timeZone || _userTimeZone
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric', month: '2-digit', day: '2-digit',
  }
  if (tz) opts.timeZone = tz

  const parts = new Intl.DateTimeFormat('en-CA', opts).formatToParts(d)
  const get = (type: string) => parts.find(p => p.type === type)?.value || ''

  return `${get('year')}-${get('month')}-${get('day')}`
}

// formatLogTime 格式化日志时间戳（RFC3339 UTC 字符串）为 "HH:mm:ss.SSS"。
// 按用户时区显示，浏览器自动处理夏令时。
export function formatLogTime(timestamp: string, timeZone?: string): string {
  if (!timestamp) return ''
  const d = new Date(timestamp)
  if (isNaN(d.getTime())) return timestamp

  const tz = timeZone || _userTimeZone
  const opts: Intl.DateTimeFormatOptions = {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }
  if (tz) opts.timeZone = tz

  const parts = new Intl.DateTimeFormat('en-CA', opts).formatToParts(d)
  const get = (type: string) => parts.find(p => p.type === type)?.value || ''
  const ms = String(d.getMilliseconds()).padStart(3, '0')

  return `${get('hour')}:${get('minute')}:${get('second')}.${ms}`
}

export function formatLatency(ms: number | null | undefined): string {
  if (ms == null) return '0s'
  const seconds = ms / 1000
  return seconds.toFixed(1) + 's'
}

export function formatTokens(tokens: number | null | undefined): string {
  if (tokens == null) return '0'
  if (tokens >= 1e9) return (tokens / 1e9).toFixed(1) + 'B'
  if (tokens >= 1e6) return (tokens / 1e6).toFixed(1) + 'M'
  if (tokens >= 1e3) return (tokens / 1e3).toFixed(1) + 'K'
  return tokens.toString()
}

export function formatToken(value: number | null | undefined): string {
  if (value == null || value === 0) return '0'
  if (value < 1000) return value.toString()
  
  if (value < 1000000) {
    const k = value / 1000
    return k % 1 === 0 ? `${k}K` : `${k.toFixed(1)}K`
  }
  
  const m = value / 1000000
  return m % 1 === 0 ? `${m}M` : `${m.toFixed(1)}M`
}

export function formatContextDisplay(context: number | null | undefined, output: number | null | undefined): string {
  const contextStr = formatToken(context)
  const outputStr = formatToken(output)
  return `${contextStr} / ${outputStr}`
}
