<template>
  <el-dialog
    v-model="visible"
    :title="t('settings.title')"
    width="580px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <!-- 个人信息 -->
    <el-card class="settings-card" shadow="never">
      <template #header>
        <div class="card-header">
          <el-icon><User /></el-icon>
          <span>{{ t('settings.profile') }}</span>
        </div>
      </template>
      <el-form :model="profile" label-width="120px">
        <el-form-item :label="t('settings.displayName')">
          <el-input v-model="profile.display_name" :placeholder="t('settings.displayNamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('settings.email')">
          <el-input v-model="profile.email" :placeholder="t('settings.emailPlaceholder')" />
          <div class="form-hint">{{ t('settings.emailHint') }}</div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="small" @click="handleSaveProfile" :loading="profileLoading">
            {{ t('settings.saveProfile') }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 偏好设置 -->
    <el-card class="settings-card" shadow="never">
      <template #header>
        <div class="card-header">
          <el-icon><Setting /></el-icon>
          <span>{{ t('settings.preferences') }}</span>
        </div>
      </template>
      <el-form label-width="120px">
        <el-form-item :label="t('settings.language')">
          <el-radio-group v-model="language" @change="handleSaveLanguage">
            <el-radio-button value="zh">中文</el-radio-button>
            <el-radio-button value="en">EN</el-radio-button>
          </el-radio-group>
          <div class="form-hint">{{ t('settings.languageHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('settings.timeZoneLabel')">
          <el-select
            v-model="selectedTz"
            filterable
            :placeholder="t('settings.timeZonePlaceholder')"
            style="width: 280px"
            @change="handleSaveTz"
          >
            <el-option :label="t('settings.timeZoneAuto') + ' (' + browserTz + ')'" value="" />
            <el-option v-for="tz in commonTimeZones" :key="tz.value" :label="tz.label" :value="tz.value" />
          </el-select>
          <div class="form-hint">{{ t('settings.timeZoneHint') }}</div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 账号安全 -->
    <el-card class="settings-card" shadow="never">
      <template #header>
        <div class="card-header">
          <el-icon><Lock /></el-icon>
          <span>{{ t('settings.security') }}</span>
        </div>
      </template>

      <el-form :model="usernameForm" :rules="usernameRules" ref="usernameFormRef" label-width="140px">
        <el-form-item :label="t('settings.username')" prop="username">
          <el-input v-model="usernameForm.username" :placeholder="userStore.user?.username || ''" />
          <div class="form-hint">{{ t('settings.usernameHint') }}</div>
        </el-form-item>
        <el-form-item :label="t('settings.currentPassword')" prop="password">
          <el-input v-model="usernameForm.password" type="password" show-password :placeholder="t('settings.passwordVerify')" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="small" @click="handleSaveUsername" :loading="usernameLoading">
            {{ t('common.save') }}
          </el-button>
        </el-form-item>
      </el-form>

      <el-divider style="margin: 12px 0" />

      <el-form :model="passwordForm" :rules="passwordRules" ref="passwordFormRef" label-width="140px">
        <el-form-item :label="t('settings.oldPassword')" prop="old_password">
          <el-input v-model="passwordForm.old_password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('settings.newPassword')" prop="new_password">
          <el-input v-model="passwordForm.new_password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('settings.confirmPassword')" prop="confirm_password">
          <el-input v-model="passwordForm.confirm_password" type="password" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="small" @click="handleChangePassword" :loading="passwordLoading">
            {{ t('settings.changePassword') }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { User, Setting, Lock } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { setUserTimeZone } from '@/utils/format'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const { t, locale } = useI18n()
const userStore = useUserStore()
const appStore = useAppStore()

// ==================== 个人信息 ====================

const profile = reactive({
  display_name: userStore.user?.display_name || '',
  email: userStore.user?.email || ''
})
const profileLoading = ref(false)

watch(visible, (v) => {
  if (v) {
    profile.display_name = userStore.user?.display_name || ''
    profile.email = userStore.user?.email || ''
    selectedTz.value = userStore.user?.time_zone || ''
  }
})

async function handleSaveProfile() {
  profileLoading.value = true
  try {
    await userStore.updateProfile({
      display_name: profile.display_name,
      email: profile.email
    })
    ElMessage.success(t('settings.profileSaved'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    profileLoading.value = false
  }
}

// ==================== 语言 ====================

const language = ref(locale.value)

async function handleSaveLanguage(val: string) {
  locale.value = val
  appStore.setLocale(val)
  try {
    await userStore.updateProfile({ language: val })
    ElMessage.success(t('settings.languageSaved'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

// ==================== 时区 ====================

const commonTimeZones = [
  { label: 'UTC', value: 'UTC' },
  { label: 'Asia/Shanghai (UTC+8)', value: 'Asia/Shanghai' },
  { label: 'Asia/Tokyo (UTC+9)', value: 'Asia/Tokyo' },
  { label: 'Asia/Seoul (UTC+9)', value: 'Asia/Seoul' },
  { label: 'Asia/Singapore (UTC+8)', value: 'Asia/Singapore' },
  { label: 'Asia/Kolkata (UTC+5:30)', value: 'Asia/Kolkata' },
  { label: 'Asia/Dubai (UTC+4)', value: 'Asia/Dubai' },
  { label: 'Europe/London', value: 'Europe/London' },
  { label: 'Europe/Paris (UTC+1/+2)', value: 'Europe/Paris' },
  { label: 'Europe/Berlin (UTC+1/+2)', value: 'Europe/Berlin' },
  { label: 'Europe/Moscow (UTC+3)', value: 'Europe/Moscow' },
  { label: 'America/New_York (UTC-5/-4)', value: 'America/New_York' },
  { label: 'America/Chicago (UTC-6/-5)', value: 'America/Chicago' },
  { label: 'America/Los_Angeles (UTC-8/-7)', value: 'America/Los_Angeles' },
  { label: 'America/Sao_Paulo (UTC-3)', value: 'America/Sao_Paulo' },
  { label: 'Australia/Sydney (UTC+10/+11)', value: 'Australia/Sydney' },
  { label: 'Pacific/Auckland (UTC+12/+13)', value: 'Pacific/Auckland' },
]

const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone
const selectedTz = ref(userStore.user?.time_zone || '')

async function handleSaveTz() {
  try {
    await userStore.updateTimeZone(selectedTz.value)
    setUserTimeZone(selectedTz.value)
    ElMessage.success(t('settings.timeZoneSaved'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

// ==================== 修改用户名 ====================

const usernameFormRef = ref()
const usernameLoading = ref(false)

const usernameForm = reactive({
  username: '',
  password: ''
})

const usernameRules = {
  username: [
    { required: true, message: () => t('common.required'), trigger: 'blur' },
    { min: 3, max: 64, message: '3-64 characters', trigger: 'blur' }
  ],
  password: [{ required: true, message: () => t('common.required'), trigger: 'blur' }]
}

async function handleSaveUsername() {
  const valid = await usernameFormRef.value.validate().catch(() => false)
  if (!valid) return

  usernameLoading.value = true
  try {
    await userStore.updateUsername(usernameForm.username, usernameForm.password)
    ElMessage.success(t('settings.usernameSaved'))
    usernameForm.username = ''
    usernameForm.password = ''
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    usernameLoading.value = false
  }
}

// ==================== 修改密码 ====================

const passwordFormRef = ref()
const passwordLoading = ref(false)

const passwordForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const passwordRules = {
  old_password: [{ required: true, message: () => t('common.required'), trigger: 'blur' }],
  new_password: [
    { required: true, message: () => t('common.required'), trigger: 'blur' },
    { min: 6, message: 'Min 6 characters', trigger: 'blur' }
  ],
  confirm_password: [
    { required: true, message: () => t('common.required'), trigger: 'blur' },
    {
      validator: (_rule: any, value: string, callback: any) => {
        if (value !== passwordForm.new_password) {
          callback(new Error(t('settings.passwordMismatch')))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}

async function handleChangePassword() {
  const valid = await passwordFormRef.value.validate().catch(() => false)
  if (!valid) return

  passwordLoading.value = true
  try {
    await userStore.changePassword(passwordForm.old_password, passwordForm.new_password)
    ElMessage.success(t('settings.passwordChanged'))
    Object.assign(passwordForm, { old_password: '', new_password: '', confirm_password: '' })
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    passwordLoading.value = false
  }
}
</script>

<style scoped>
.settings-card {
  margin-bottom: 12px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
}

.card-header .el-icon {
  color: var(--el-color-primary);
}

.form-hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-top: 3px;
}
</style>
