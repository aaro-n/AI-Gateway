<template>
  <div class="login-page">
    <div class="lang-top-right">
      <el-dropdown trigger="click" @command="changeLocale">
        <span class="lang-toggle">
          🌐 {{ locale === 'zh' ? '中文' : 'English' }}
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="zh" :class="{ active: locale === 'zh' }">
              <span class="lang-flag">🇨🇳</span> 中文 (Chinese)
            </el-dropdown-item>
            <el-dropdown-item command="en" :class="{ active: locale === 'en' }">
              <span class="lang-flag">🇺🇸</span> English
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
    <div class="login-card">
      <h1 class="title">{{ t('login.title') }}</h1>
      <el-form :model="form" :rules="rules" ref="formRef" @submit.prevent="handleLogin">
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            :placeholder="t('login.username')"
            size="large"
            prefix-icon="User"
          />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            :placeholder="t('login.password')"
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
            class="login-btn"
          >
            {{ t('login.submit') }}
          </el-button>
        </el-form-item>
      </el-form>
      <div class="forgot-link">
        <router-link to="/forgot-password">{{ t('login.forgotPassword') }}</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'

const { t, locale } = useI18n()
const router = useRouter()
const userStore = useUserStore()
const appStore = useAppStore()

const formRef = ref()
const loading = ref(false)

const form = reactive({
  username: '',
  password: ''
})

const rules = {
  username: [{ required: true, message: () => t('login.username'), trigger: 'blur' }],
  password: [{ required: true, message: () => t('login.password'), trigger: 'blur' }]
}

async function handleLogin() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const success = await userStore.login(form.username, form.password)
    if (success) {
      router.push('/')
    } else {
      ElMessage.error(t('login.invalidCredentials'))
    }
  } finally {
    loading.value = false
  }
}

function changeLocale(lang: string) {
  locale.value = lang
  appStore.setLocale(lang)
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f5f5;
  position: relative;
}

.lang-top-right {
  position: absolute;
  top: 16px;
  right: 24px;
  z-index: 100;
}

.lang-toggle {
  cursor: pointer;
  color: var(--el-text-color-regular);
  font-size: 14px;
  padding: 6px 10px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.lang-toggle:hover {
  background-color: var(--el-fill-color-light);
}

.lang-flag {
  margin-right: 4px;
}

.active {
  color: var(--el-color-primary);
  font-weight: bold;
}

.login-card {
  width: 400px;
  padding: 40px;
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

.title {
  text-align: center;
  margin-bottom: 30px;
  font-size: 24px;
  color: var(--el-text-color-primary);
}

.login-btn {
  width: 100%;
}

.forgot-link {
  text-align: center;
  margin-bottom: 16px;
}

.forgot-link a {
  color: var(--el-color-primary);
  text-decoration: none;
  font-size: 13px;
}
</style>
