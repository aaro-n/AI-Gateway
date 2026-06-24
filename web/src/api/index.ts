import axios from 'axios'
import { useUserStore } from '@/stores/user'
import { trackApiError } from '@/core/apiErrorTracker'

const api = axios.create({
  baseURL: '/api/v1',
  withCredentials: true
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error.response?.status || 0
    const url = error.config?.url || 'unknown'
    const method = error.config?.method?.toUpperCase() || 'UNKNOWN'
    const message = error.response?.data?.error?.message
      || error.response?.data?.error
      || error.message
      || 'Unknown error'

    // 追踪错误
    trackApiError(url, method, status, String(message))

    if (status === 401) {
      const userStore = useUserStore()
      userStore.user = null
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  }
)

export default api
