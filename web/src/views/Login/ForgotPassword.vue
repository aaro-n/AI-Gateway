<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="title">{{ t('login.resetPassword') }}</h1>
      <p class="subtitle">{{ t('login.sendResetEmail') }}</p>
      <el-form :model="form" :rules="rules" ref="formRef" @submit.prevent="handleSubmit">
        <el-form-item prop="email">
          <el-input
            v-model="form.email"
            :placeholder="t('settings.emailPlaceholder')"
            size="large"
            prefix-icon="Message"
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
            {{ t('login.sendResetEmail') }}
          </el-button>
        </el-form-item>
      </el-form>
      <p v-if="sent" class="success-msg">{{ t('login.resetLinkSent') }}</p>
      <div class="back-link">
        <router-link to="/login">{{ t('login.backToLogin') }}</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@/stores/user'

const { t } = useI18n()
const userStore = useUserStore()

const formRef = ref()
const loading = ref(false)
const sent = ref(false)

const form = reactive({
  email: ''
})

const rules = {
  email: [
    { required: true, message: 'Required', trigger: 'blur' },
    { type: 'email', message: 'Invalid email', trigger: 'blur' }
  ]
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await userStore.forgotPassword(form.email)
    sent.value = true
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
