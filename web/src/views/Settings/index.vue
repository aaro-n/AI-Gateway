<template>
  <div class="settings-page">
    <h2 class="page-title">{{ t('settings.title') }}</h2>

    <!-- Tab: SMTP + 用户管理（admin only） -->
    <el-tabs v-model="activeTab" type="card">
      <el-tab-pane label="SMTP" name="smtp">
        <!-- 发送测试邮件 -->
        <el-card class="settings-card" shadow="never">
          <template #header>
            <div class="card-header">
              <el-icon><Promotion /></el-icon>
              <span>{{ t('settings.smtpTestTitle') }}</span>
            </div>
          </template>

          <p class="page-desc">{{ t('settings.smtpTestHint') }}</p>

          <el-form :model="testForm" label-width="140px" style="max-width: 560px">
            <el-form-item :label="t('settings.smtpTestTo')" prop="to">
              <el-input v-model="testForm.to" :placeholder="userEmail" />
              <div class="form-hint">{{ t('settings.smtpTestToHint') }}</div>
            </el-form-item>

            <el-form-item :label="t('settings.smtpTestSubject')" prop="subject">
              <el-input v-model="testForm.subject" :placeholder="defaultSubject" />
            </el-form-item>

            <el-form-item :label="t('settings.smtpTestBody')" prop="body">
              <el-input
                v-model="testForm.body"
                type="textarea"
                :rows="4"
                :placeholder="t('settings.smtpTestBodyPlaceholder')"
              />
            </el-form-item>

            <el-form-item>
              <el-button type="primary" @click="handleTest" :loading="testLoading" :icon="Promotion">
                {{ t('settings.smtpTest') }}
              </el-button>
            </el-form-item>
          </el-form>

          <div v-if="testResult !== null" style="margin-top: 12px">
            <el-alert
              :type="testResult.success ? 'success' : 'error'"
              :title="testResult.success ? t('settings.smtpTestSuccess') : t('settings.smtpTestFail')"
              :description="testResult.message"
              :closable="false"
              show-icon
            />
          </div>
        </el-card>

        <!-- 域名信息 -->
        <el-card class="settings-card" shadow="never">
          <template #header>
            <div class="card-header">
              <el-icon><Link /></el-icon>
              <span>{{ t('settings.smtpDomainTitle') }}</span>
            </div>
          </template>
          <p class="page-desc">{{ t('settings.smtpDomainHint') }}</p>
          <div style="display: flex; align-items: center; gap: 8px; max-width: 560px">
            <el-input v-model="currentDomain" readonly style="flex: 1" />
            <el-tag type="info" size="small">{{ t('settings.smtpDomainAuto') }}</el-tag>
          </div>
        </el-card>
      </el-tab-pane>

      <!-- 用户管理 Tab（仅管理员可见） -->
      <el-tab-pane v-if="isAdmin" label="用户管理" name="users">
        <el-card class="settings-card" shadow="never">
          <template #header>
            <div class="card-header">
              <el-icon><UserFilled /></el-icon>
              <span>用户管理</span>
              <el-button type="primary" size="small" @click="showUserDialog()" style="margin-left: auto">
                添加用户
              </el-button>
            </div>
          </template>

          <el-table :data="users" stripe v-loading="userLoading">
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="username" label="用户名" min-width="120" />
            <el-table-column prop="display_name" label="显示名" min-width="100" />
            <el-table-column prop="role" label="角色" width="100">
              <template #default="{ row }">
                <el-tag :type="row.role === 'admin' ? 'danger' : 'info'" size="small">
                  {{ row.role === 'admin' ? '管理员' : '普通用户' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="250">
              <template #default="{ row }">
                <el-button link type="primary" @click="showUserDialog(row.id)">编辑</el-button>
                <el-button link type="success" @click="showPermissionsDialog(row)">权限</el-button>
                <el-button link type="danger" @click="handleDeleteUser(row)"
                  :disabled="row.role === 'admin'">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- 用户创建/编辑对话框 -->
    <el-dialog v-model="userDialogVisible" :title="editingUserId ? '编辑用户' : '添加用户'" width="500px" destroy-on-close>
      <el-form :model="userForm" ref="userFormRef" label-width="100px">
        <el-form-item label="用户名" prop="username" :rules="[{ required: true, message: '请输入用户名' }]">
          <el-input v-model="userForm.username" placeholder="登录用户名" :disabled="!!editingUserId" />
        </el-form-item>
        <el-form-item label="显示名" prop="display_name">
          <el-input v-model="userForm.display_name" placeholder="显示名称（可选）" />
        </el-form-item>
        <el-form-item v-if="!editingUserId" label="密码" prop="password" :rules="[{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '密码至少6位', trigger: 'blur' }]">
          <el-input v-model="userForm.password" type="password" show-password placeholder="用户密码" />
        </el-form-item>
        <el-form-item v-if="editingUserId" label="新密码" prop="password" :rules="[{ min: 6, message: '密码至少6位', trigger: 'blur' }]">
          <el-input v-model="userForm.password" type="password" show-password placeholder="留空则不修改" />
        </el-form-item>
        <el-form-item v-if="editingUserId" label="状态">
          <el-switch v-model="userForm.enabled" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSaveUser" :loading="userSaving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 权限编辑对话框 -->
    <el-dialog v-model="permsDialogVisible" :title="`权限管理 — ${permsUser?.username}`" width="700px" destroy-on-close>
      <div v-loading="permsLoading">
        <h4>可访问的模型厂商</h4>
        <el-select
          v-model="permsProviderIds"
          multiple
          filterable
          placeholder="请选择厂商"
          style="width: 100%; margin-bottom: 20px;"
        >
          <el-option
            v-for="p in allProviders"
            :key="p.id"
            :label="p.name"
            :value="p.id"
          />
        </el-select>

        <h4>可访问的模型映射</h4>
        <el-select
          v-model="permsModelIds"
          multiple
          filterable
          placeholder="请选择模型"
          style="width: 100%;"
        >
          <el-option
            v-for="m in allModels"
            :key="m.id"
            :label="m.model || m.slug"
            :value="m.id"
          />
        </el-select>
      </div>
      <template #footer>
        <el-button @click="permsDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSavePermissions" :loading="permsSaving">保存权限</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Promotion, Link, UserFilled } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import api from '@/api'

const { t } = useI18n()
const userStore = useUserStore()

const isAdmin = computed(() => userStore.isAdmin)

// ===================== Tab =====================
const activeTab = ref('smtp')

// ===================== SMTP =====================
const testLoading = ref(false)
const testResult = ref<{ success: boolean; message: string } | null>(null)

const userEmail = computed(() => userStore.user?.email || '')

const defaultSubject = `AI Gateway - ${t('settings.smtpTest')}`

const form = reactive({
  enabled: false,
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  use_tls: false
})

const testForm = reactive({
  to: '',
  subject: '',
  body: ''
})

const currentDomain = ref('')

// ===================== 用户管理 =====================
const users = ref<any[]>([])
const userLoading = ref(false)
const userDialogVisible = ref(false)
const editingUserId = ref<number | null>(null)
const userSaving = ref(false)
const userFormRef = ref()

const userForm = reactive({
  username: '',
  display_name: '',
  password: '',
  enabled: true
})

const permsDialogVisible = ref(false)
const permsUser = ref<any>(null)
const permsLoading = ref(false)
const permsSaving = ref(false)
const permsProviderIds = ref<number[]>([])
const permsModelIds = ref<number[]>([])
const allProviders = ref<any[]>([])
const allModels = ref<any[]>([])

// ===================== Lifecycle =====================
onMounted(async () => {
  // SMTP
  try {
    const smtp = await userStore.fetchSMTPConfig()
    if (smtp) {
      form.enabled = smtp.enabled
      form.host = smtp.host || ''
      form.port = smtp.port || 587
      form.username = smtp.username || ''
      form.password = smtp.password || ''
      form.from = smtp.from || ''
      form.use_tls = smtp.use_tls || false
    }
  } catch (e: any) {
    // 非管理员或请求失败，静默处理
  }
  currentDomain.value = window.location.origin
  testForm.to = userEmail.value

  // 用户管理
  if (isAdmin.value) {
    fetchUsers()
  }
})

// ===================== SMTP Methods =====================
async function handleTest() {
  if (!testForm.to.trim()) {
    ElMessage.warning(t('settings.smtpTestToRequired'))
    return
  }
  testLoading.value = true
  testResult.value = null
  try {
    const result = await userStore.testSMTP({
      host: form.host, port: form.port,
      username: form.username, password: form.password,
      from: form.from, use_tls: form.use_tls,
      to: testForm.to.trim(),
      subject: testForm.subject || defaultSubject,
      body: testForm.body
    })
    testResult.value = { success: result.success, message: result.message || result.error || '' }
  } catch (e: any) {
    testResult.value = { success: false, message: e.response?.data?.error || e.message || 'Unknown error' }
  } finally {
    testLoading.value = false
  }
}

// ===================== 用户管理 Methods =====================
async function fetchUsers() {
  userLoading.value = true
  try {
    const res = await api.get('/admin/users')
    users.value = res.data.users || []
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '获取用户列表失败')
  } finally {
    userLoading.value = false
  }
}

function showUserDialog(id?: number) {
  editingUserId.value = id || null
  userForm.username = ''
  userForm.display_name = ''
  userForm.password = ''
  userForm.enabled = true
  if (id) {
    const u = users.value.find((x: any) => x.id === id)
    if (u) {
      userForm.username = u.username
      userForm.display_name = u.display_name || ''
      userForm.enabled = u.enabled !== false
    }
  }
  userDialogVisible.value = true
}

async function handleSaveUser() {
  userSaving.value = true
  try {
    const payload: any = {
      username: userForm.username,
      display_name: userForm.display_name
    }
    if (userForm.password) payload.password = userForm.password
    if (editingUserId.value) {
      payload.enabled = userForm.enabled
      await api.put(`/admin/users/${editingUserId.value}`, payload)
      ElMessage.success('用户更新成功')
    } else {
      await api.post('/admin/users', payload)
      ElMessage.success('用户创建成功')
    }
    userDialogVisible.value = false
    fetchUsers()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  } finally {
    userSaving.value = false
  }
}

async function handleDeleteUser(row: any) {
  if (row.role === 'admin') return
  try {
    await ElMessageBox.confirm(`确定要删除用户「${row.username}」吗？该操作将同时删除该用户的所有 API 密钥和相关数据。`, '确认删除', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消'
    })
  } catch { return }

  try {
    await api.delete(`/admin/users/${row.id}`)
    ElMessage.success('用户已删除')
    fetchUsers()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

async function showPermissionsDialog(user: any) {
  permsUser.value = user
  permsProviderIds.value = []
  permsModelIds.value = []
  permsDialogVisible.value = true
  permsLoading.value = true
  try {
    const [permRes, provRes, modelRes] = await Promise.all([
      api.get(`/admin/users/${user.id}/permissions`),
      api.get('/providers'),
      api.get('/models')
    ])
    const permData = permRes.data.permissions || {}
    permsProviderIds.value = (permData.providers || []).map((p: any) => p.id)
    permsModelIds.value = (permData.models || []).map((m: any) => m.id)
    allProviders.value = provRes.data.providers || provRes.data || []
    allModels.value = modelRes.data.models || modelRes.data || []
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '获取权限失败')
  } finally {
    permsLoading.value = false
  }
}

async function handleSavePermissions() {
  permsSaving.value = true
  try {
    await api.put(`/admin/users/${permsUser.value.id}/permissions`, {
      provider_ids: permsProviderIds.value,
      model_ids: permsModelIds.value
    })
    ElMessage.success('权限保存成功')
    permsDialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '权限保存失败')
  } finally {
    permsSaving.value = false
  }
}
</script>

<style scoped>
.settings-page {
  padding: 20px;
  max-width: 960px;
}

.page-title {
  font-size: 22px;
  font-weight: 600;
  margin: 0 0 20px 0;
  color: var(--el-text-color-primary);
}

.page-desc {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  margin-bottom: 16px;
}

/* el-tabs card 类型：标签页之间增加水平间距 */
.settings-page :deep(.el-tabs--card > .el-tabs__header .el-tabs__item) {
  margin-right: 4px;
  border-radius: 6px 6px 0 0;
}

/* 标签头部与内容区域增加垂直间距 */
.settings-page :deep(.el-tabs__header) {
  margin-bottom: 24px;
}

.settings-card {
  margin-bottom: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
}

.card-header .el-icon {
  font-size: 18px;
  color: var(--el-color-primary);
}

.form-hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.inline-hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
