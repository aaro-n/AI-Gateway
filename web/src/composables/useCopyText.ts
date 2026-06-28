import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'

export function useCopyText() {
  const { t } = useI18n()

  function copy(text: string | undefined | null) {
    if (!text) return
    // 优先使用 Clipboard API（需要安全上下文）
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        ElMessage.success(t('common.copied'))
      }).catch(() => {
        ElMessage.error(t('common.error'))
      })
      return
    }
    // 非安全上下文（如 HTTP IP 访问）降级方案
    try {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const success = document.execCommand('copy')
      document.body.removeChild(textarea)
      if (success) {
        ElMessage.success(t('common.copied'))
      } else {
        ElMessage.error(t('common.error'))
      }
    } catch {
      ElMessage.error(t('common.error'))
    }
  }

  return { copy }
}
