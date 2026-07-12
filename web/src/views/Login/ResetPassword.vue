<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="title">{{ t('login.resetPassword') }}</h1>
      <p class="subtitle">输入新密码完成重置</p>
      <el-form :model="form" :rules="rules" ref="formRef" @submit.prevent="handleSubmit">
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            :placeholder="t('login.newPassword')"
            size="large"
            prefix-icon="Lock"
            show-password
          />
        </el-form-item>
        <el-form-item prop="confirm_password">
          <el-input
            v-model="form.confirm_password"
            type="password"
            :placeholder="t('login.confirmPassword')"
            size="large"
            prefix-icon="Lock"
            show-password
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            native-type="submit"
            :loading="loading"
            class="submit-btn"
          >
            {{ t('login.resetSubmit') }}
          </el-button>
        </el-form-item>
      </el-form>
      <p v-if="success" class="success-msg">{{ t('login.resetSuccess') }}</p>
      <div class="back-link">
        <router-link to="/login">{{ t('login.backToLogin') }}</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const formRef = ref()
const loading = ref(false)
const success = ref(false)

const token = (route.query.token as string) || ''

const form = reactive({
  password: '',
  confirm_password: ''
})

const rules = {
  password: [
    { required: true, message: 'Required', trigger: 'blur' },
    { min: 6, message: 'Min 6 characters', trigger: 'blur' }
  ],
  confirm_password: [
    { required: true, message: 'Required', trigger: 'blur' },
    {
      validator: (_rule: any, value: string, callback: any) => {
        if (value !== form.password) {
          callback(new Error('Passwords do not match'))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}

async function handleSubmit() {
  if (!token) {
    return
  }
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await userStore.resetPassword(token, form.password)
    success.value = true
    setTimeout(() => { router.push('/login') }, 2000)
  } catch (e: any) {
    // error is handled by interceptor
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page { min-height: 100vh; display: flex; align-items: center; justify-content: center; background: #f5f5f5; }
.login-card { width: 400px; padding: 40px; background: #fff; border-radius: 8px; box-shadow: 0 2px 12px rgba(0,0,0,0.1); }
.title { text-align: center; margin-bottom: 8px; font-size: 24px; color: var(--el-text-color-primary); }
.subtitle { text-align: center; color: #909399; font-size: 14px; margin-bottom: 24px; }
.submit-btn { width: 100%; }
.success-msg { text-align: center; color: #67c23a; font-size: 14px; }
.back-link { text-align: center; margin-top: 16px; }
.back-link a { color: var(--el-color-primary); text-decoration: none; font-size: 14px; }
</style>
