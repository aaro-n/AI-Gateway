<template>
  <div class="settings-page">
    <!-- 时区设置 -->
    <el-card style="margin-bottom: 16px">
      <template #header>{{ t('settings.timeZone') }}</template>
      <div style="max-width: 500px">
        <p style="color: #909399; font-size: 13px; margin-bottom: 12px;">
          {{ t('settings.timeZoneHint') }}
        </p>
        <el-select
          v-model="selectedTz"
          filterable
          :placeholder="t('settings.timeZonePlaceholder')"
          style="width: 340px"
        >
          <el-option
            :label="t('settings.timeZoneAuto') + ' (' + browserTz + ')'"
            value=""
          />
          <el-option
            v-for="tz in commonTimeZones"
            :key="tz.value"
            :label="tz.label"
            :value="tz.value"
          >
            <span>{{ tz.label }}</span>
            <span v-if="tz.value === currentTz" style="float: right; color: #909399; font-size: 12px">{{ t('settings.timeZoneCurrent') }}</span>
          </el-option>
        </el-select>
        <el-button type="primary" style="margin-left: 12px" @click="handleSaveTz" :loading="tzLoading">{{ t('common.save') }}</el-button>
        <p v-if="tzSaved" style="color: #67c23a; font-size: 13px; margin-top: 8px">{{ t('settings.timeZoneSaved') }}</p>
      </div>
    </el-card>

    <!-- 修改密码 -->
    <el-card>
      <template #header>{{ t('settings.changePassword') }}</template>
      <el-form :model="form" :rules="rules" ref="formRef" label-width="150px" style="max-width: 400px">
        <el-form-item :label="t('settings.oldPassword')" prop="old_password">
          <el-input v-model="form.old_password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('settings.newPassword')" prop="new_password">
          <el-input v-model="form.new_password" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('settings.confirmPassword')" prop="confirm_password">
          <el-input v-model="form.confirm_password" type="password" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSubmit" :loading="loading">{{ t('common.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { setUserTimeZone } from '@/utils/format'

const { t } = useI18n()
const userStore = useUserStore()

// 常见时区列表
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

// 浏览器本地时区
const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone

// 当前用户时区
const currentTz = computed(() => userStore.timeZone)

// 选择框值：空字符串 = 使用浏览器时区
const selectedTz = ref(userStore.timeZone || '')
const tzLoading = ref(false)
const tzSaved = ref(false)

async function handleSaveTz() {
  tzLoading.value = true
  tzSaved.value = false
  try {
    await userStore.updateTimeZone(selectedTz.value)
    setUserTimeZone(selectedTz.value)
    tzSaved.value = true
    ElMessage.success(t('settings.timeZoneSaved'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    tzLoading.value = false
  }
}

// 密码修改相关

const formRef = ref()
const loading = ref(false)

const form = reactive({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const rules = {
  old_password: [{ required: true, message: 'Required', trigger: 'blur' }],
  new_password: [
    { required: true, message: 'Required', trigger: 'blur' },
    { min: 6, message: 'Min 6 characters', trigger: 'blur' }
  ],
  confirm_password: [
    { required: true, message: 'Required', trigger: 'blur' },
    {
      validator: (_rule: any, value: string, callback: any) => {
        if (value !== form.new_password) {
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
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await userStore.changePassword(form.old_password, form.new_password)
    ElMessage.success(t('settings.passwordChanged'))
    Object.assign(form, { old_password: '', new_password: '', confirm_password: '' })
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.settings-page { padding: 20px; }
</style>
