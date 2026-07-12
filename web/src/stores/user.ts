import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'

export interface User {
  id: number
  username: string
  role: string
  time_zone: string
}

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const loading = ref(false)

  const isLoggedIn = computed(() => !!user.value)
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

  return {
    user,
    loading,
    isLoggedIn,
    username,
    timeZone,
    login,
    logout,
    fetchUser,
    changePassword,
    updateTimeZone
  }
})
