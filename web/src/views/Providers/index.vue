<template>
  <div class="providers">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('menu.providers') }}</span>
          <div class="header-actions">
            <el-button type="danger" @click="handleBatchDelete" :disabled="selectedIds.length === 0">{{ t('common.batchDelete') }} ({{ selectedIds.length }})</el-button>
            <el-button type="primary" @click="showDialog()">{{ t('provider.addProvider') }}</el-button>
          </div>
        </div>
      </template>
      <el-table :data="providers" stripe v-loading="loading" @selection-change="handleSelectionChange" :default-sort="defaultSort" @sort-change="handleSortChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="name" :label="t('provider.name')" width="220" sortable />
        <el-table-column :label="t('provider.apiStyles')">
          <template #default="{ row }">
            <el-tag v-if="row.openai_base_url" type="success" style="margin-right: 4px">OpenAI</el-tag>
            <el-tag v-if="row.anthropic_base_url" type="primary" style="margin-right: 4px">Anthropic</el-tag>
            <el-tag v-if="row.gemini_base_url" type="warning" style="margin-right: 4px">Gemini</el-tag>
            <el-tag v-if="row.deepseek_base_url" type="danger">DeepSeek</el-tag>
            <span v-if="!row.openai_base_url && !row.anthropic_base_url && !row.gemini_base_url && !row.deepseek_base_url">-</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('provider.models')" width="120" prop="models" sortable :sort-method="(a: any, b: any) => sortByArrayLength(a, b, 'models')">
          <template #default="{ row }">
            {{ row.models?.length || 0 }}
          </template>
        </el-table-column>
        <el-table-column :label="t('common.status')" width="120" prop="enabled" sortable>
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleEnabled(row)" />
          </template>
        </el-table-column>
        <el-table-column :label="t('common.action')" width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDialog(row.id)">{{ t('common.edit') }}</el-button>
            <el-button link type="default" @click="goDetail(row.id)">{{ t('common.detail') }}</el-button>
            <el-button link type="danger" @click="handleDelete(row.id)">{{ t('common.delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingId ? t('provider.editProvider') : t('provider.addProvider')" width="650px" destroy-on-close>
      <el-form :model="form" :rules="rules" ref="formRef" label-width="160px" v-loading="dialogLoading">
        
        <!-- 1. 厂商类型选择 -->
        <el-form-item label="厂商类型" required>
          <el-select v-model="form.provider_type" placeholder="请选择厂商类型" @change="onTypeChange" :disabled="!!editingId" style="width: 100%">
            <el-option
              v-for="item in protocolMeta"
              :key="item.name"
              :label="item.name === 'openai' ? 'OpenAI' : item.name === 'anthropic' ? 'Anthropic' : item.name === 'gemini' ? 'Google Gemini' : item.name === 'deepseek' ? 'DeepSeek' : item.name.toUpperCase()"
              :value="item.name"
            />
          </el-select>
        </el-form-item>

        <!-- 2. 厂商名称（选厂商类型后才显示，必填）-->
        <el-form-item v-if="form.provider_type" label="厂商名称" prop="name" required>
          <el-input v-model="form.name" placeholder="请输入厂商显示名称，如：我的 OpenAI、公司 Gemini 代理" maxlength="50" show-word-limit />
        </el-form-item>

        <!-- 3. 代理地址（选厂商类型后才显示）-->
        <el-form-item v-if="form.provider_type" label="代理地址" required>
          <el-input v-model="form.base_url" :placeholder="baseUrlPlaceholder" />
        </el-form-item>

        <!-- 4. API 密钥（选厂商类型后才显示；编辑时留空不修改）-->
        <el-form-item v-if="form.provider_type" label="API 密钥" prop="api_key" :required="!editingId">
          <el-input v-model="form.api_key" type="password" show-password :placeholder="editingId ? '留空则不修改密钥' : '请输入 API 密钥'" />
        </el-form-item>

        <!-- 5. 测试连接 -->
        <el-form-item v-if="form.provider_type" label=" ">
          <el-button type="success" @click="handleTestConnection" :loading="testingConnection" :disabled="!form.base_url">
            测试连接是否通畅
          </el-button>
        </el-form-item>

        <!-- 6. 模型配置（选厂商类型后才显示）-->
        <div v-if="form.provider_type" class="models-section">
          <div class="models-section-header">
            <span class="models-section-title">渠道模型配置</span>
            <div class="models-section-actions">
              <el-button type="primary" size="small" @click="handleFetchProviderModels" :loading="testingConnection">获取厂商模型列表</el-button>
              <el-button type="success" size="small" @click="testAllFetchedModels" :loading="testingAllModels" :disabled="fetchedModels.length === 0">测试全部</el-button>
              <el-button type="danger" size="small" plain @click="removeFailedModels" :disabled="!hasFailedModels">删除未通过</el-button>
            </div>
          </div>

          <!-- 手动添加 Model ID -->
          <div class="custom-model-input">
            <el-input v-model="customModelId" placeholder="输入 Model ID 手动添加" size="default" @keyup.enter="addCustomModelId" />
            <el-button type="primary" @click="addCustomModelId">添加</el-button>
          </div>

          <!-- 模型列表 -->
          <el-table :data="fetchedModels" size="small" border max-height="250" style="width: 100%;">
            <el-table-column prop="model_id" label="Model ID" min-width="160" show-overflow-tooltip />
            <el-table-column label="状态" width="160">
              <template #default="{ row }">
                <el-tag v-if="row.testStatus === 'untested'" type="info" size="small">未测试</el-tag>
                <el-tag v-else-if="row.testStatus === 'success'" type="success" size="small">可用 {{ row.latency }}ms</el-tag>
                <el-tooltip v-else-if="row.testStatus === 'failed'" :content="row.error" placement="top">
                  <el-tag type="danger" size="small" style="cursor: pointer;">不可用</el-tag>
                </el-tooltip>
                <el-tag v-else-if="row.testStatus === 'testing'" type="warning" size="small">测试中...</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="来源" width="70">
              <template #default="{ row }">
                <el-tag v-if="row.source === 'sync'" type="info" size="small">Sync</el-tag>
                <el-tag v-else type="warning" size="small">Manual</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row, $index }">
                <el-button link type="success" size="small" @click="testSingleFetchedModel(row)" :loading="row.testStatus === 'testing'">测试</el-button>
                <el-button link type="danger" size="small" @click="removeFetchedModel($index)">移除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

      </el-form>
      <template #footer>
        <!-- 6. 最后是保存数据按钮 -->
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/api'
import { getSortConfig, setSortConfig, sortByArrayLength } from '@/utils/tableSort'

const { t } = useI18n()
const router = useRouter()

const providers = ref<any[]>([])
const selectedIds = ref<number[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const dialogLoading = ref(false)
const editingId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref()
const defaultSort = getSortConfig('providers', 'name')

const form = reactive({
  name: '',
  openai_base_url: '',
  anthropic_base_url: '',
  gemini_base_url: '',
  deepseek_base_url: '',
  base_url: '', // Unified BaseURL input shown to user, mapped dynamically
  api_key: '',
  provider_type: ''
})

const rules = computed(() => ({
  name: [{ required: true, message: 'Required', trigger: 'blur' }],
  api_key: editingId.value ? [] : [{ required: true, message: 'Required', trigger: 'blur' }]
}))

const selectedPreset = ref('')
const protocolMeta = ref<any[]>([])
const testingConnection = ref(false)
const fetchedModels = ref<any[]>([])
const customModelId = ref('')
const testingAllModels = ref(false)

const baseUrlPlaceholder = computed(() => {
  const selected = protocolMeta.value.find(p => p.name === form.provider_type)
  return selected ? selected.default_base_url : '请输入渠道代理地址'
})

function validateBaseURL() {
  if (!form.base_url) {
    ElMessage.error('端点地址 (Base URL) 不能为空')
    return false
  }
  return true
}

onMounted(() => {
  fetchProviders()
  fetchProtocolsMeta()
})

const testConcurrency = ref(5) // 默认 5，从服务端 API 读取

async function fetchProtocolsMeta() {
  try {
    const res = await api.get('/protocols')
    protocolMeta.value = res.data.protocols || []
    testConcurrency.value = res.data.test_concurrency || 5
  } catch (e) {
    console.error('Failed to load protocol metadata', e)
  }
}

function onTypeChange(val: string) {
  // 切换厂商类型时自动填入默认 Base URL，但不自动填名称
  const selected = protocolMeta.value.find(p => p.name === val)
  if (selected) {
    form.base_url = selected.default_base_url
    mapBaseUrls()
  }
}

function mapBaseUrls() {
  // 只更新当前类型的 URL，不覆盖其他已配置的 URL（避免编辑时丢失多 BaseURL 数据）
  if (form.provider_type === 'openai') {
    form.openai_base_url = form.base_url
  } else if (form.provider_type === 'anthropic') {
    form.anthropic_base_url = form.base_url
  } else if (form.provider_type === 'gemini') {
    form.gemini_base_url = form.base_url
  } else if (form.provider_type === 'deepseek') {
    form.deepseek_base_url = form.base_url
  }
}

// Watch base_url and provider_type dynamically to keep them 100% in sync reactively!
watch(() => [form.base_url, form.provider_type], () => {
  mapBaseUrls()
}, { deep: true })

function addFastModel(name: string) {
  if (fetchedModels.value.some(m => m.model_id === name)) {
    ElMessage.warning('Model ID already in list')
    return
  }
  fetchedModels.value.push({
    model_id: name,
    testStatus: 'untested',
    latency: 0,
    error: '',
    source: 'manual'
  })
}

function addCustomModelId() {
  const id = customModelId.value.trim()
  if (!id) return
  if (fetchedModels.value.some(m => m.model_id === id)) {
    ElMessage.warning('Model ID already in the list')
    return
  }
  fetchedModels.value.push({
    model_id: id,
    testStatus: 'untested',
    latency: 0,
    error: '',
    source: 'manual'
  })
  customModelId.value = ''
}

// 待删除的模型 ID（提交时从数据库删除）
const pendingDeleteIds = ref<number[]>([])

function removeFetchedModel(idx: number) {
  const m = fetchedModels.value[idx]
  if (m._exists && m.id) {
    pendingDeleteIds.value.push(m.id)
  }
  fetchedModels.value.splice(idx, 1)
}

function removeFailedModels() {
  const before = fetchedModels.value.length
  const failed = fetchedModels.value.filter(m => m.testStatus === 'failed')
  for (const m of failed) {
    if (m._exists && m.id) pendingDeleteIds.value.push(m.id)
  }
  fetchedModels.value = fetchedModels.value.filter(m => m.testStatus !== 'failed')
  ElMessage.success(`已删除 ${before - fetchedModels.value.length} 个未通过的模型ID`)
}

async function testSingleFetchedModel(model: any) {
  model.testStatus = 'testing'
  try {
    const res = await api.post('/providers/test-model', {
      provider_id: editingId.value || 0,
      openai_base_url: form.openai_base_url,
      anthropic_base_url: form.anthropic_base_url,
      gemini_base_url: form.gemini_base_url,
      deepseek_base_url: form.deepseek_base_url,
      api_key: form.api_key || 'DUMMY_KEY_FOR_EDIT',
      model_id: model.model_id
    })
    const tests = res.data.tests || []
    const failed = tests.some((t: any) => !t.success)
    if (tests.length > 0 && !failed) {
      model.testStatus = 'success'
      model.latency = tests[0].latency_ms
    } else {
      model.testStatus = 'failed'
      model.error = tests.find((t: any) => t.error)?.error || '测试失败'
    }
  } catch (e: any) {
    model.testStatus = 'failed'
    model.error = e.response?.data?.error || '连接服务器超时'
  }
}

const hasFailedModels = computed(() =>
  fetchedModels.value.some(m => m.testStatus === 'failed')
)

async function testAllFetchedModels() {
  const models = fetchedModels.value
  if (models.length === 0) return
  testingAllModels.value = true
  models.forEach(m => { m.testStatus = 'untested'; m.latency = 0; m.error = '' })
  try {
    const limit = testConcurrency.value
    let index = 0

    async function worker() {
      while (index < models.length) {
        const currentIndex = index++
        await testSingleFetchedModelAsync(models[currentIndex], currentIndex)
      }
    }

    // 启动 limit 个 worker，任一完成自动取下一个
    const workers = []
    const count = Math.min(limit, models.length)
    for (let i = 0; i < count; i++) {
      workers.push(worker())
    }
    await Promise.all(workers)

    const ok = models.filter(m => m.testStatus === 'success').length
    const fail = models.filter(m => m.testStatus === 'failed').length
    ElMessage.success(`测试完成：${ok} 通过，${fail} 未通过`)
  } finally {
    testingAllModels.value = false
  }
}

// Internal async version that doesn't throw, used by testAll
async function testSingleFetchedModelAsync(model: any, idx: number) {
  model.testStatus = 'testing'
  try {
    const res = await api.post('/providers/test-model', {
      provider_id: editingId.value || 0,
      openai_base_url: form.openai_base_url,
      anthropic_base_url: form.anthropic_base_url,
      gemini_base_url: form.gemini_base_url,
      deepseek_base_url: form.deepseek_base_url,
      api_key: form.api_key || 'DUMMY_KEY_FOR_EDIT',
      model_id: model.model_id
    })
    const tests = res.data.tests || []
    const failed = tests.some((t: any) => !t.success)
    if (tests.length > 0 && !failed) {
      model.testStatus = 'success'
      model.latency = tests[0].latency_ms
    } else {
      model.testStatus = 'failed'
      model.error = tests.find((t: any) => t.error)?.error || '测试失败'
    }
  } catch (e: any) {
    model.testStatus = 'failed'
    model.error = e.response?.data?.error || '连接服务器超时'
  }
}

async function handleFetchProviderModels() {
  if (!validateBaseURL()) return
  if (!form.api_key && !editingId.value) {
    ElMessage.error('API Key is required to fetch models')
    return
  }
  testingConnection.value = true
  try {
    const res = await api.post('/providers/test-connection', {
      openai_base_url: form.openai_base_url,
      anthropic_base_url: form.anthropic_base_url,
      gemini_base_url: form.gemini_base_url,
      deepseek_base_url: form.deepseek_base_url,
      api_key: form.api_key
    })
    const list = res.data.models || []
    // 合并：已有模型保留，只添加新模型
    const existing = new Set(fetchedModels.value.map(m => m.model_id))
    for (const m of list) {
      const modelID = m.model_id || m.ModelID || ''
      if (!modelID || existing.has(modelID)) continue
      
      const displayName = m.display_name || m.DisplayName || modelID
      const ownedBy = m.owned_by || m.OwnedBy || form.provider_type
      const contextWindow = m.context_window !== undefined ? m.context_window : (m.ContextWindow !== undefined ? m.ContextWindow : 4096)
      const maxOutput = m.max_output !== undefined ? m.max_output : (m.MaxOutput !== undefined ? m.MaxOutput : 1024)
      const supportsVision = m.supports_vision !== undefined ? m.supports_vision : (m.SupportsVision !== undefined ? m.SupportsVision : true)
      const supportsTools = m.supports_tools !== undefined ? m.supports_tools : (m.SupportsTools !== undefined ? m.SupportsTools : true)
      const supportsStream = m.supports_stream !== undefined ? m.supports_stream : (m.SupportsStream !== undefined ? m.SupportsStream : true)

      fetchedModels.value.push({
        model_id: modelID,
        display_name: displayName,
        owned_by: ownedBy,
        context_window: contextWindow,
        max_output: maxOutput,
        supports_vision: supportsVision,
        supports_tools: supportsTools,
        supports_stream: supportsStream,
        testStatus: 'untested',
        latency: 0,
        error: '',
        source: 'sync'
      })
      existing.add(modelID)
    }
    ElMessage.success(`成功拉取到该厂商 ${list.length} 个原生模型列表。你可以对感兴趣的模型点击“测试”进行可用性探测，也可以自行添加 Model ID。`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '获取模型列表失败')
  } finally {
    testingConnection.value = false
  }
}

async function handleTestConnection() {
  if (!validateBaseURL()) return
  if (!form.api_key && !editingId.value) {
    ElMessage.error('API Key is required to test connection')
    return
  }
  testingConnection.value = true
  try {
    await api.post('/providers/test-connection', {
      openai_base_url: form.openai_base_url,
      anthropic_base_url: form.anthropic_base_url,
      gemini_base_url: form.gemini_base_url,
      deepseek_base_url: form.deepseek_base_url,
      api_key: form.api_key
    })
    ElMessage.success('连接测试成功！厂商端点及 API Key 校验通过，可以配置和测试模型。')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '连接测试失败')
  } finally {
    testingConnection.value = false
  }
}

async function fetchProviders() {
  loading.value = true
  try {
    const res = await api.get('/providers')
    providers.value = res.data.providers || []
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    loading.value = false
  }
}

function handleSelectionChange(selection: any[]) {
  selectedIds.value = selection.map(item => item.id)
}

async function showDialog(id?: number) {
  editingId.value = id || null
  selectedPreset.value = ''
  fetchedModels.value = []
  pendingDeleteIds.value = []
  Object.assign(form, { name: '', openai_base_url: '', anthropic_base_url: '', gemini_base_url: '', deepseek_base_url: '', base_url: '', api_key: '', provider_type: '' })
  dialogVisible.value = true
  
  if (id) {
    dialogLoading.value = true
    try {
      const res = await api.get(`/providers/${id}`)
      const provider = res.data.provider
      if (provider) {
        const type = provider.openai_base_url ? 'openai' : (provider.anthropic_base_url ? 'anthropic' : (provider.gemini_base_url ? 'gemini' : (provider.deepseek_base_url ? 'deepseek' : '')))
        const bUrl = provider.openai_base_url || provider.anthropic_base_url || provider.gemini_base_url || provider.deepseek_base_url || ''
        Object.assign(form, {
          name: provider.name || '',
          openai_base_url: provider.openai_base_url || '',
          anthropic_base_url: provider.anthropic_base_url || '',
          gemini_base_url: provider.gemini_base_url || '',
          deepseek_base_url: provider.deepseek_base_url || '',
          base_url: bUrl,
          api_key: '',
          provider_type: type
        })

        // 加载已有模型到对话框列表（标记 _exists，保存时不重复创建）
        const models = provider.models || []
        fetchedModels.value = models.map((m: any) => ({
          id: m.id,
          model_id: m.model_id,
          testStatus: m.is_available ? 'success' : 'untested',
          latency: 0,
          error: '',
          source: m.source || 'manual',
          _exists: true
        }))

        // 获取最近的测试结果，更新状态和延迟
        try {
          const trRes = await api.get(`/providers/${id}/test-results`)
          const results = trRes.data.results || []
          const resultMap: Record<string, any> = {}
          for (const r of results) {
            resultMap[r.model_id] = r
          }
          for (const m of fetchedModels.value) {
            const r = resultMap[m.model_id]
            if (r) {
              m.latency = r.latency_ms || 0
              m.error = r.error || ''
              if (!r.success) {
                m.testStatus = 'failed'
              } else if (m.testStatus === 'untested') {
                m.testStatus = 'success'
              }
            }
          }
        } catch (e) {
          // 测试结果获取失败，使用默认状态
        }
      }
    } catch (e) {
      ElMessage.error(t('common.error'))
      dialogVisible.value = false
    } finally {
      dialogLoading.value = false
    }
  }
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  
  if (!validateBaseURL()) return

  // Make sure BaseURLs are populated based on form.base_url and provider_type before submit
  mapBaseUrls()

  submitting.value = true
  try {
    let providerId = editingId.value
    if (editingId.value) {
      await api.put(`/providers/${editingId.value}`, form)
    } else {
      const res = await api.post('/providers', form)
      providerId = res.data.provider.id
    }

    // 删除在对话框中移除的已有模型
    if (pendingDeleteIds.value.length > 0 && providerId) {
      for (const delId of pendingDeleteIds.value) {
        await api.delete(`/providers/${providerId}/models/${delId}`).catch(() => {})
      }
    }

    // 保存模型中新增的（_exists 标记为 false 或未标记 = 对话框里新加的）
    const newModels = fetchedModels.value.filter(m => !m._exists)
    const failedModels: string[] = []
    if (newModels.length > 0 && providerId) {
      for (const m of newModels) {
        try {
          await api.post(`/providers/${providerId}/models`, {
            model_id: m.model_id,
            display_name: m.display_name || m.model_id,
            owned_by: m.owned_by || form.provider_type,
            context_window: m.context_window !== undefined ? m.context_window : 4096,
            max_output: m.max_output !== undefined ? m.max_output : 1024,
            supports_vision: m.supports_vision !== false,
            supports_tools: m.supports_tools !== false,
            supports_stream: m.supports_stream !== false,
            source: m.source || 'manual'
          })
        } catch (err: any) {
          failedModels.push(m.model_id)
        }
      }
    }

    if (failedModels.length > 0) {
      ElMessage.warning(`保存成功，但以下模型 ID 添加失败：${failedModels.join(', ')}`)
    } else {
      ElMessage.success(t('common.success'))
    }
    dialogVisible.value = false
    fetchProviders()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(id: number) {
  await ElMessageBox.confirm(t('common.confirm'), t('common.delete'), { type: 'warning' })
  await api.delete(`/providers/${id}`)
  ElMessage.success(t('common.success'))
  fetchProviders()
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return
  await ElMessageBox.confirm(t('common.confirm') + ` (${selectedIds.value.length} items)`, t('common.batchDelete'), { type: 'warning' })
  try {
    await Promise.all(selectedIds.value.map(id => api.delete(`/providers/${id}`)))
    ElMessage.success(t('common.success'))
    selectedIds.value = []
    fetchProviders()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function toggleEnabled(row: any) {
  const previousEnabled = !row.enabled
  try {
    await api.put(`/providers/${row.id}`, { enabled: row.enabled })
  } catch (e: any) {
    row.enabled = previousEnabled // 回滚开关状态
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

function goDetail(id: number) {
  router.push(`/providers/${id}`)
}

function handleSortChange({ prop, order }: any) {
  if (prop && order) {
    setSortConfig('providers', { prop, order })
  }
}
</script>

<style scoped>
.providers {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.models-section {
  margin-top: 16px;
  border-top: 1px solid var(--el-border-color-light);
  padding-top: 16px;
}

.models-section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.models-section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.models-section-actions {
  display: flex;
  gap: 8px;
}

.preset-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin-bottom: 12px;
}

.preset-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-right: 4px;
}

.custom-model-input {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
</style>
