<template>
  <div class="keys-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('menu.keys') }}</span>
          <div class="header-actions">
            <el-button type="danger" @click="handleBatchDelete" :disabled="selectedIds.length === 0">{{ t('common.batchDelete') }} ({{ selectedIds.length }})</el-button>
            <el-button type="primary" @click="showDialog()">{{ t('key.createKey') }}</el-button>
          </div>
        </div>
      </template>
      <el-table :data="keys" stripe v-loading="loading" @selection-change="handleSelectionChange" :default-sort="defaultSort" @sort-change="handleSortChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="name" :label="t('key.name')" width="180" sortable />
        <el-table-column prop="key" :label="t('key.key')" width="280">
          <template #default="{ row }">
            <div style="display: flex; align-items: center;">
              <span>{{ row.key }}</span>
              <el-tag
                v-if="getKeyProviderLabel(row)"
                :type="getKeyProviderType(row)"
                size="small"
                effect="plain"
                style="margin-left: 16px;"
              >{{ getKeyProviderLabel(row) }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('key.model')" prop="models" sortable :sort-method="(a: any, b: any) => sortByArrayLength(a, b, 'models')">
          <template #default="{ row }">
            <span v-if="!row.models || row.models.length === 0" style="color: #999">{{ t('key.allModels') }}</span>
            <span v-else>{{ t('key.allowedCount', { count: row.models.length }) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('mcp.tools')" prop="mcp_tools_count" sortable>
          <template #default="{ row }">
            <span v-if="row.mcp_tools_count === 0" style="color: #999">{{ t('key.allModels') }}</span>
            <span v-else>{{ t('key.allowedCount', { count: row.mcp_tools_count }) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('mcp.resources')" prop="mcp_resources_count" sortable>
          <template #default="{ row }">
            <span v-if="row.mcp_resources_count === 0" style="color: #999">{{ t('key.allModels') }}</span>
            <span v-else>{{ t('key.allowedCount', { count: row.mcp_resources_count }) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('mcp.prompts')" prop="mcp_prompts_count" sortable>
          <template #default="{ row }">
            <span v-if="row.mcp_prompts_count === 0" style="color: #999">{{ t('key.allModels') }}</span>
            <span v-else>{{ t('key.allowedCount', { count: row.mcp_prompts_count }) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.status')" width="150" prop="enabled" sortable>
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleEnabled(row)" />
          </template>
        </el-table-column>
        <el-table-column :label="t('common.action')" width="240">
           <template #default="{ row }">
             <el-button link type="primary" @click="showDialog(row)">{{ t('common.edit') }}</el-button>
             <el-button link type="warning" @click="handleUpdateKey(row)">更新密钥</el-button>
             <el-button link type="default" @click="goDetail(row.slug || row.id)">{{ t('common.detail') }}</el-button>
             <el-button link type="danger" @click="handleDelete(row.id)">{{ t('common.delete') }}</el-button>
           </template>
         </el-table-column>
      </el-table>
    </el-card>

<el-dialog v-model="dialogVisible" :title="editingId ? t('common.edit') : t('key.createKey')" width="500px">
       <el-form :model="form" ref="formRef" label-width="auto">
         <el-form-item :label="t('key.name')" required>
           <el-input v-model="form.name" />
         </el-form-item>
         <el-form-item :label="t('key.format')" required>
           <el-select v-model="form.format" :placeholder="editingId ? undefined : t('key.selectFormat')" style="width: 100%" :disabled="!!editingId">
             <el-option
               v-for="p in protocols"
               :key="p.name"
               :label="p.label"
               :value="p.name"
             />
           </el-select>
         </el-form-item>

       </el-form>
       <template #footer>
         <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
         <el-button type="primary" @click="handleSubmit" :loading="submitting">{{ t('common.save') }}</el-button>
       </template>
     </el-dialog>

    <el-dialog v-model="keyDialogVisible" title="密钥已更新">
      <div v-if="oldKey" style="margin-bottom:12px">
        <p style="color:#909399;font-size:13px">原密钥:</p>
        <el-input :model-value="oldKey" readonly disabled />
      </div>
      <p>新密钥:</p>
      <el-input v-model="newKey" readonly>
        <template #append>
          <el-button @click="copyKey">Copy</el-button>
        </template>
      </el-input>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useCopyText } from '@/composables/useCopyText'
import api from '@/api'
import { getSortConfig, setSortConfig, sortByArrayLength } from '@/utils/tableSort'

const { t } = useI18n()
const router = useRouter()

const { copy } = useCopyText()

const keys = ref<any[]>([])
const selectedIds = ref<number[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const keyDialogVisible = ref(false)
const newKey = ref('')
const oldKey = ref('')
const editingId = ref<number | null>(null)
const formRef = ref()
const submitting = ref(false)
const defaultSort = getSortConfig('keys', 'name')
const protocols = ref<Array<{ name: string; label: string; key_prefix: string }>>([])

const form = reactive({
  name: '',
  access_mode: 'mapping',
  format: ''
})

onMounted(() => {
  fetchKeys()
  fetchProtocols()
})

async function fetchKeys() {
  loading.value = true
  try {
    const res = await api.get('/keys')
    keys.value = res.data.keys || []
  } finally {
    loading.value = false
  }
}

async function fetchProtocols() {
  try {
    const res = await api.get('/protocols')
    protocols.value = res.data.protocols || []
  } catch (e) {
    console.error('Failed to fetch protocols', e)
  }
}

function handleSelectionChange(selection: any[]) {
  selectedIds.value = selection.map(item => item.id)
}

async function showDialog(key?: any) {
  editingId.value = key?.id || null
  
  if (key) {
    form.name = key.name || ''
    form.access_mode = key.access_mode || 'mapping'
    // 编辑时显示当前格式，但不可修改
    const fmtKeys = key.formats ? Object.keys(key.formats) : []
    form.format = fmtKeys.length > 0 ? fmtKeys[0] : ''
  } else {
    form.name = ''
    form.access_mode = 'mapping'
    form.format = protocols.value.length > 0 ? protocols.value[0].name : ''
  }
  
  dialogVisible.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editingId.value) {
      // 编辑模式：仅更新名称
      await api.put(`/keys/${editingId.value}`, { name: form.name, access_mode: form.access_mode })
      ElMessage.success(t('common.success'))
      dialogVisible.value = false
    } else {
      const res = await api.post('/keys', { name: form.name, access_mode: form.access_mode, format: form.format })
      newKey.value = res.data.raw_key
      oldKey.value = ''
      dialogVisible.value = false
      keyDialogVisible.value = true
    }
    fetchKeys()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    submitting.value = false
  }
}

// 更新密钥：直接调用reset接口生成新密钥
async function handleUpdateKey(row: any) {
  try {
    await ElMessageBox.confirm(`确定要更新密钥【${row.key}】吗？更新后旧密钥将失效。`, '更新密钥', { type: 'warning' })
    // 根据密钥前缀检测格式
    let format = 'openai'
    if (row.key && row.key.startsWith('AIza')) {
      format = 'gemini'
    } else if (row.key && row.key.startsWith('sk-ant-')) {
      format = 'anthropic'
    } else if (row.key && row.key.startsWith('sk-')) {
      format = 'openai'
    }
    
    const resetRes = await api.post(`/keys/${row.id}/reset`, { format })
    oldKey.value = row.key  // 显示脱敏后的原密钥
    newKey.value = resetRes.data.raw_key
    keyDialogVisible.value = true
    fetchKeys()
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error(e.response?.data?.error || t('common.error'))
    }
  }
}

const providerLabels: Record<string, { label: string; type: string }> = {
  openai: { label: 'OpenAI', type: 'success' },
  anthropic: { label: 'Anthropic', type: 'primary' },
  gemini: { label: 'Gemini', type: 'warning' },
  deepseek: { label: 'DeepSeek', type: 'danger' },
}

function getKeyProviderLabel(row: any): string {
  const fmtKeys = row.formats ? Object.keys(row.formats) : []
  const fmt = fmtKeys[0]
  if (!fmt) return ''
  return providerLabels[fmt]?.label || fmt.toUpperCase()
}

function getKeyProviderType(row: any): string {
  const fmtKeys = row.formats ? Object.keys(row.formats) : []
  const fmt = fmtKeys[0]
  if (!fmt) return 'info'
  return providerLabels[fmt]?.type || 'info'
}

async function toggleEnabled(row: any) {
  await api.put(`/keys/${row.id}`, { enabled: row.enabled })
}

function copyKey() {
  copy(newKey.value)
}

function goDetail(idOrSlug: string | number) {
  router.push(`/keys/${idOrSlug}`)
}

function handleSortChange({ prop, order }: any) {
  if (prop && order) {
    setSortConfig('keys', { prop, order })
  }
}

async function handleDelete(id: number) {
  await ElMessageBox.confirm(t('common.confirm'), t('common.delete'), { type: 'warning' })
  await api.delete(`/keys/${id}`)
  ElMessage.success(t('common.success'))
  fetchKeys()
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return
  await ElMessageBox.confirm(t('common.confirm') + ` (${selectedIds.value.length} items)`, t('common.batchDelete'), { type: 'warning' })
  try {
    await Promise.all(selectedIds.value.map(id => api.delete(`/keys/${id}`)))
    ElMessage.success(t('common.success'))
    selectedIds.value = []
    fetchKeys()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}
</script>

<style scoped>
.keys-page { padding: 20px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.header-actions { display: flex; gap: 10px; }
</style>
