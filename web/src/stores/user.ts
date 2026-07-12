import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'

export interface User {
  id: number
  username: string
  display_name: string
  email: string
  role: string
  time_zone: string
  language: string
}

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const loading = ref(false)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const username = computed(() => user.value?.username || '')
  // 用户时区（IANA 时区名）。空值表示使用浏览器本地时区。
  const timeZone = computed(() => user.value?.time_zone || '')

  async function login(username: string, password: string) {
    loading.value = true
    try {
      const res = await api.post('/auth/login', { username, password })
      user.value = res.data.user
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  async function logout() {
    await api.post('/auth/logout')
    user.value = null
  }

  async function fetchUser() {
    try {
      const res = await api.get('/auth/me')
      user.value = res.data.user
    } catch {
      user.value = null
    }
  }

  async function changePassword(oldPassword: string, newPassword: string) {
    await api.put('/auth/password', { old_password: oldPassword, new_password: newPassword })
  }

  async function updateTimeZone(tz: string) {
    await api.put('/auth/timezone', { time_zone: tz })
    if (user.value) {
      user.value.time_zone = tz
    }
  }

  async function updateProfile(fields: { display_name?: string; email?: string; language?: string }) {
    const res = await api.put('/auth/profile', fields)
    if (user.value) {
      if (fields.display_name !== undefined) user.value.display_name = res.data.display_name
      if (fields.email !== undefined) user.value.email = res.data.email
      if (fields.language !== undefined) user.value.language = res.data.language
    }
    return res.data
  }

  async function updateUsername(username: string, password: string) {
    const res = await api.put('/auth/username', { username, password })
    if (user.value) {
      user.value.username = res.data.username
    }
    return res.data
  }

  async function forgotPassword(email: string) {
    return api.post('/auth/forgot-password', { email })
  }

  async function resetPassword(token: string, password: string) {
    return api.post('/auth/reset-password', { token, password })
  }

  async function fetchSMTPConfig() {
    const res = await api.get('/admin/smtp')
    return res.data.smtp
  }

  async function saveSMTPConfig(config: {
    enabled: boolean; host: string; port: number; username: string; password: string; from: string; use_tls: boolean
  }) {
    const res = await api.post('/admin/smtp', config)
    return res.data
  }

  async function testSMTP(params: {
    host: string; port: number; username: string; password: string; from: string; use_tls: boolean;
    to: string; subject: string; body: string
  }) {
    const res = await api.post('/admin/smtp/test', params)
    return res.data
  }

  return {
    user,
    loading,
    isLoggedIn,
    isAdmin,
    username,
    timeZone,
    login,
    logout,
    fetchUser,
    changePassword,
    updateTimeZone,
    updateProfile,
    updateUsername,
    forgotPassword,
    resetPassword,
    fetchSMTPConfig,
    saveSMTPConfig,
    testSMTP
  }
})
