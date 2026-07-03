<template>
  <div class="key-detail">
    <el-page-header @back="goBack" :title="t('menu.keys')">
      <template #content>
        <span class="text-large font-600 mr-3">{{ key?.name || '-' }}</span>
      </template>
    </el-page-header>

    <el-card class="info-card" v-loading="loading">
      <el-descriptions :column="2" border>
        <el-descriptions-item :label="t('key.name')">{{ key?.name }}</el-descriptions-item>
        <el-descriptions-item :label="t('common.createdAt')">
          {{ key?.created_at ? new Date(key.created_at).toLocaleString() : '-' }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('common.status')">
          <el-tag :type="key?.enabled ? 'success' : 'info'" size="small">
            {{ key?.enabled ? t('common.enabled') : t('common.disabled') }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="t('key.expiresAt')">
          {{ key?.expires_at ? new Date(key.expires_at).toLocaleString() : '-' }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('key.key')">
          <div style="display: flex; align-items: center; gap: 12px;">
            <code>{{ key?.key }}</code>
            <el-tag v-if="keyProtocolLabel" :type="keyProtocolType" size="small" effect="plain">{{ keyProtocolLabel }}</el-tag>
          </div>
        </el-descriptions-item>
      </el-descriptions>
      <div class="actions">
        <el-button @click="handleReset" :loading="resetting">{{ t('key.reset') }}</el-button>
      </div>
    </el-card>

    <el-card class="tabs-card">
      <el-tabs v-model="activeTab">
        <el-tab-pane label="模型厂商" name="providers">
          <div class="tab-actions">
            <el-button @click="enableAllProviderModels" :loading="enablingProviderModels" :disabled="!key?.enabled">{{ t('key.allowAll') }}</el-button>
            <el-button @click="disableAllProviderModels" :loading="disablingProviderModels" :disabled="!key?.enabled">全部禁用</el-button>
          </div>
          <el-table :data="providerModels" stripe v-loading="providerModelsLoading" :default-sort="providersDefaultSort" @sort-change="(e: any) => handleSortChange('key-providers', e)">
            <el-table-column prop="provider_name" label="厂商名称" width="120" sortable />
            <el-table-column prop="model_id" label="模型ID" width="180" sortable />
            <el-table-column prop="display_name" label="显示名称" width="140" sortable />
            <el-table-column prop="owned_by" label="归属" width="80" sortable />
            <el-table-column :label="t('models.contextWindow')" width="100" prop="context_window" sortable>
              <template #default="{ row }">
                <span v-if="row.context_window > 0">{{ row.context_window.toLocaleString() }}</span>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.status')" width="70" prop="enabled" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" content="密钥已禁用" placement="top">
                  <el-switch :model-value="row.enabled" disabled />
                </el-tooltip>
                <el-switch v-else :model-value="row.enabled" @update:model-value="toggleProviderModel(row)" />
              </template>
            </el-table-column>
            <el-table-column :label="t('common.action')" width="70">
              <template #default="{ row }">
                <el-button link type="danger" size="small" @click="removeProviderModel(row)" :disabled="!key?.enabled">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="margin-top: 12px;">
            <el-button type="primary" @click="showProviderModelDialog" :disabled="!key?.enabled">添加直通模型</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane label="模型映射" name="models">
          <div class="tab-actions">
            <el-button @click="enableAllModels" :loading="enablingModels" :disabled="!key?.enabled">{{ t('key.allowAll') }}</el-button>
            <el-button @click="disableAllModels" :loading="disablingModels" :disabled="!key?.enabled">全部禁用</el-button>
          </div>
          <el-table :data="models" stripe v-loading="modelsLoading" :default-sort="modelsDefaultSort" @sort-change="(e: any) => handleSortChange('key-models', e)">
            <el-table-column prop="name" :label="t('models.name')" width="160" sortable />
            <el-table-column :label="t('models.mappingCount')" width="90" prop="mapping_count" sortable>
              <template #default="{ row }">
                {{ row.mapping_count || 0 }}
              </template>
            </el-table-column>
            <el-table-column :label="t('models.capabilities')" width="140">
              <template #default="{ row }">
                <div class="capability-tags">
                  <el-tag v-if="row.supports_stream" type="primary" size="small" style="margin-right: 4px">Stream</el-tag>
                  <el-tag v-if="row.supports_tools" type="warning" size="small" style="margin-right: 4px">Tools</el-tag>
                  <el-tag v-if="row.supports_vision" type="success" size="small">Vision</el-tag>
                  <span v-if="!row.supports_stream && !row.supports_tools && !row.supports_vision">-</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="t('models.contextWindow')" width="110" prop="min_context_window" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="row.min_context_window > 0 || row.min_max_output > 0"
                  :content="`${row.min_context_window?.toLocaleString() || 0} / ${row.min_max_output?.toLocaleString() || 0}`"
                  placement="top">
                  <span>{{ formatContextDisplay(row.min_context_window, row.min_max_output) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.status')" width="70" prop="enabled" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" :content="t('key.keyDisabled')" placement="top">
                  <el-switch :model-value="row.enabled" disabled />
                </el-tooltip>
                <el-switch v-else :model-value="row.enabled" @update:model-value="toggleModel(row)" />
              </template>
            </el-table-column>
            <el-table-column :label="t('common.action')" width="70">
              <template #default="{ row }">
                <el-button link type="danger" size="small" @click="removeModel(row)" :disabled="!key?.enabled">{{ t('common.delete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="margin-top: 12px;">
            <el-button type="primary" @click="showModelDialog" :disabled="!key?.enabled">添加模型映射</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('mcp.tools')" name="tools">
          <div class="tab-actions">
            <el-button @click="clearTools" :loading="clearingTools" :disabled="!key?.enabled">{{ t('key.allowAll') }}</el-button>
          </div>
          <el-table :data="tools" stripe v-loading="toolsLoading" :default-sort="toolsDefaultSort" @sort-change="(e: any) => handleSortChange('key-tools', e)">
            <el-table-column :label="t('mcp.toolName')" width="300" prop="name" sortable>
              <template #default="{ row }">
                <div>{{ row.mcp_name }}.{{ row.name }}</div>
              </template>
            </el-table-column>
            <el-table-column :label="t('mcp.description')" prop="description" sortable>
              <template #default="{ row }">
                <div class="description-cell">
                  <div class="description-text" :class="{ expanded: row._expanded }">
                    {{ row.description || '-' }}
                  </div>
                  <div class="description-actions">
                    <el-button v-if="row.description && isLongText(row.description)" link type="primary" size="small" @click="row._expanded = !row._expanded">
                      {{ row._expanded ? t('common.collapse') : t('common.expand') }}
                    </el-button>
                    <CopyButton v-if="row.description" :text="row.description" />
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.status')" width="180" prop="selected" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" :content="t('key.keyDisabled')" placement="top">
                  <el-radio-group v-model="row.selected" disabled>
                    <el-radio :label="false">{{ t('key.default') }}</el-radio>
                    <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                  </el-radio-group>
                </el-tooltip>
                <el-radio-group v-else v-model="row.selected" @change="toggleTool(row)">
                  <el-radio :label="false">{{ t('key.default') }}</el-radio>
                  <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                </el-radio-group>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('mcp.resources')" name="resources">
          <div class="tab-actions">
            <el-button @click="clearResources" :loading="clearingResources" :disabled="!key?.enabled">{{ t('key.allowAll') }}</el-button>
          </div>
          <el-table :data="resources" stripe v-loading="resourcesLoading" :default-sort="resourcesDefaultSort" @sort-change="(e: any) => handleSortChange('key-resources', e)">
            <el-table-column :label="t('mcp.resourceName')" width="300" prop="name" sortable>
              <template #default="{ row }">
                <div>{{ row.mcp_name }}.{{ row.name }}</div>
              </template>
            </el-table-column>
            <el-table-column :label="t('mcp.description')" prop="description" sortable>
              <template #default="{ row }">
                <div class="description-cell">
                  <div class="description-text" :class="{ expanded: row._expanded }">
                    {{ row.description || '-' }}
                  </div>
                  <div class="description-actions">
                    <el-button v-if="row.description && isLongText(row.description)" link type="primary" size="small" @click="row._expanded = !row._expanded">
                      {{ row._expanded ? t('common.collapse') : t('common.expand') }}
                    </el-button>
                    <CopyButton v-if="row.description" :text="row.description" />
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="uri" :label="t('mcp.resourceUri')" width="300" sortable />
            <el-table-column prop="mime_type" label="MIME Type" width="200" sortable />
            <el-table-column :label="t('common.status')" width="180" prop="selected" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" :content="t('key.keyDisabled')" placement="top">
                  <el-radio-group v-model="row.selected" disabled>
                    <el-radio :label="false">{{ t('key.default') }}</el-radio>
                    <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                  </el-radio-group>
                </el-tooltip>
                <el-radio-group v-else v-model="row.selected" @change="toggleResource(row)">
                  <el-radio :label="false">{{ t('key.default') }}</el-radio>
                  <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                </el-radio-group>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('mcp.prompts')" name="prompts">
          <div class="tab-actions">
            <el-button @click="clearPrompts" :loading="clearingPrompts" :disabled="!key?.enabled">{{ t('key.allowAll') }}</el-button>
          </div>
          <el-table :data="prompts" stripe v-loading="promptsLoading" :default-sort="promptsDefaultSort" @sort-change="(e: any) => handleSortChange('key-prompts', e)">
            <el-table-column :label="t('mcp.promptName')" width="300" prop="name" sortable>
              <template #default="{ row }">
                <div>{{ row.mcp_name }}.{{ row.name }}</div>
              </template>
            </el-table-column>
            <el-table-column :label="t('mcp.description')" prop="description" sortable>
              <template #default="{ row }">
                <div class="description-cell">
                  <div class="description-text" :class="{ expanded: row._expanded }">
                    {{ row.description || '-' }}
                  </div>
                  <div class="description-actions">
                    <el-button v-if="row.description && isLongText(row.description)" link type="primary" size="small" @click="row._expanded = !row._expanded">
                      {{ row._expanded ? t('common.collapse') : t('common.expand') }}
                    </el-button>
                    <CopyButton v-if="row.description" :text="row.description" />
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.status')" width="180" prop="selected" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" :content="t('key.keyDisabled')" placement="top">
                  <el-radio-group v-model="row.selected" disabled>
                    <el-radio :label="false">{{ t('key.default') }}</el-radio>
                    <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                  </el-radio-group>
                </el-tooltip>
                <el-radio-group v-else v-model="row.selected" @change="togglePrompt(row)">
                  <el-radio :label="false">{{ t('key.default') }}</el-radio>
                  <el-radio :label="true">{{ t('key.allowOnly') }}</el-radio>
                </el-radio-group>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>

  <el-dialog v-model="keyDialogVisible" :title="t('key.key')" width="500px">
    <p>{{ t('key.key') }}:</p>
    <el-input v-model="newKey" readonly>
      <template #append>
        <el-button @click="copyKey">{{ t('common.copied') }}</el-button>
      </template>
    </el-input>
  </el-dialog>

  <!-- 添加直通模型选择框 -->
  <el-dialog v-model="providerModelDialogVisible" title="添加直通模型" width="650px">
    <el-form label-width="auto" v-loading="providerModelDialogLoading">
      <el-form-item label="厂商名称" required>
        <el-select v-model="directProviderForm.provider_id" @change="onDirectProviderChange" placeholder="请选择厂商" filterable style="width: 100%">
          <el-option v-for="p in directProviders" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
      </el-form-item>
    </el-form>
    <div v-if="directProviderForm.provider_id" style="margin-bottom: 8px;">
      <el-input v-model="directModelSearch" placeholder="搜索模型ID..." clearable style="margin-bottom: 8px;" />
      <el-table
        ref="directModelTableRef"
        :data="paginatedDirectModels"
        stripe
        max-height="340"
        @selection-change="onDirectModelSelectionChange"
        row-key="id"
      >
        <el-table-column type="selection" width="45" :selectable="() => true" />
        <el-table-column label="模型ID" prop="model_id" min-width="200">
          <template #default="{ row }">
            <code style="font-size: 13px;">{{ row.model_id }}</code>
          </template>
        </el-table-column>
        <el-table-column label="显示名称" prop="display_name" min-width="160">
          <template #default="{ row }">
            <span style="color: var(--el-text-color-secondary); font-size: 12px;">{{ row.display_name }}</span>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="filteredDirectModels.length === 0" description="无可用模型" :image-size="60" />
    </div>
    <template #footer>
      <el-button @click="providerModelDialogVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="addSelectedProviderModels" :loading="addingProviderModels" :disabled="directSelectedModelIDs.length === 0">
        {{ t('common.save') }} ({{ directSelectedModelIDs.length }})
      </el-button>
    </template>
  </el-dialog>

  <!-- 添加模型映射选择框 -->
  <el-dialog v-model="modelDialogVisible" title="添加模型映射" width="600px">
    <el-input v-model="modelSearch" placeholder="搜索模型名称..." clearable style="margin-bottom: 12px;" />
    <el-table :data="filteredAvailableModels" stripe max-height="400" @selection-change="handleModelSelectionChange">
      <el-table-column type="selection" width="50" />
      <el-table-column prop="name" :label="t('models.name')" width="200" />
      <el-table-column :label="t('models.mappingCount')" width="120" prop="mapping_count" />
      <el-table-column :label="t('models.capabilities')">
        <template #default="{ row }">
          <div class="capability-tags">
            <el-tag v-if="row.supports_stream" type="primary" size="small" style="margin-right: 4px">Stream</el-tag>
            <el-tag v-if="row.supports_tools" type="warning" size="small" style="margin-right: 4px">Tools</el-tag>
            <el-tag v-if="row.supports_vision" type="success" size="small">Vision</el-tag>
          </div>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button @click="modelDialogVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="addSelectedModels" :loading="addingModels" :disabled="selectedModelIDs.length === 0">
        {{ t('common.save') }} ({{ selectedModelIDs.length }})
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useCopyText } from '@/composables/useCopyText'
import CopyButton from '@/components/CopyButton.vue'
import api from '@/api'
import { formatContextDisplay } from '@/utils/format'
import { getSortConfig, setSortConfig } from '@/utils/tableSort'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()

const { copy } = useCopyText()

const keyId = route.params.id as string
const key = ref<any>(null)
const loading = ref(false)
const resetting = ref(false)
const activeTab = ref('providers')
const keyDialogVisible = ref(false)
const newKey = ref('')

const providerLabels: Record<string, { label: string; type: string }> = {
  openai: { label: 'OpenAI', type: 'success' },
  anthropic: { label: 'Anthropic', type: 'primary' },
  gemini: { label: 'Gemini', type: 'warning' },
  deepseek: { label: 'DeepSeek', type: 'danger' },
  openrouter: { label: 'OpenRouter', type: '' },
}

const keyProtocolLabel = computed(() => {
  // 直接读取后端返回的 format 字段
  if (!key.value?.format) return ''
  return providerLabels[key.value.format]?.label || key.value.format.toUpperCase()
})

const keyProtocolType = computed((): 'success' | 'primary' | 'warning' | 'danger' | 'info' => {
  if (!key.value?.format) return 'info'
  return (providerLabels[key.value.format]?.type || 'info') as any
})

const models = ref<any[]>([])
const providers = ref<any[]>([])
const providerModels = ref<any[]>([])
const tools = ref<any[]>([])
const resources = ref<any[]>([])
const prompts = ref<any[]>([])

const modelsLoading = ref(false)
const providersLoading = ref(false)
const providerModelsLoading = ref(false)
const toolsLoading = ref(false)
const resourcesLoading = ref(false)
const promptsLoading = ref(false)

const enablingModels = ref(false)
const enablingProviderModels = ref(false)
const disablingProviderModels = ref(false)
const disablingModels = ref(false)
const clearingTools = ref(false)
const clearingResources = ref(false)
const clearingPrompts = ref(false)

// 直通模型选择弹窗
const providerModelDialogVisible = ref(false)
const providerModelDialogLoading = ref(false)
const addingProviderModels = ref(false)
const directProviders = ref<any[]>([])
const directAvailableModels = ref<any[]>([])
const directModelSearch = ref('')
const directSelectedModelIDs = ref<number[]>([])
const directModelTableRef = ref<any>(null)
const availableModelsForDialog = ref<any[]>([])
const directProviderForm = reactive({
  provider_id: null as number | null,
})

const filteredDirectModels = computed(() => {
  const s = directModelSearch.value.toLowerCase()
  if (!s) return directAvailableModels.value
  return directAvailableModels.value.filter((m: any) =>
    m.model_id?.toLowerCase().includes(s) || m.display_name?.toLowerCase().includes(s)
  )
})

const paginatedDirectModels = computed(() => filteredDirectModels.value)

// 模型映射选择弹窗
const modelDialogVisible = ref(false)
const modelSearch = ref('')
const selectedModelIDs = ref<number[]>([])
const addingModels = ref(false)

const modelsDefaultSort = getSortConfig('key-models', 'name')
const providersDefaultSort = getSortConfig('key-providers', 'name')
let directProvidersLoaded = false
const toolsDefaultSort = getSortConfig('key-tools', 'name')
const resourcesDefaultSort = getSortConfig('key-resources', 'name')
const promptsDefaultSort = getSortConfig('key-prompts', 'name')

onMounted(() => {
  fetchKey()
})

// 切换 key 时重置厂商加载缓存，以匹配新 key 的协议格式
watch(() => route.params.id, () => {
  directProvidersLoaded = false
})

watch(activeTab, (newTab) => {
  if (newTab === 'providers' && providerModels.value.length === 0) fetchProviderModels()
  if (newTab === 'models' && models.value.length === 0) fetchModelTab()
  if (newTab === 'tools' && tools.value.length === 0) fetchTools()
  if (newTab === 'resources' && resources.value.length === 0) fetchResources()
  if (newTab === 'prompts' && prompts.value.length === 0) fetchPrompts()
})



async function fetchKey() {
  loading.value = true
  try {
    const res = await api.get(`/keys/${keyId}`)
    key.value = res.data.key
    fetchProviderModels()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
    goBack()
  } finally {
    loading.value = false
  }
}

async function fetchModelTab() {
  modelsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/models`)
    models.value = (res.data.models || []).filter((m: any) => m.selected)
  } finally {
    modelsLoading.value = false
  }
}

async function fetchProviderModels() {
  providerModelsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/provider-models`)
    providerModels.value = (res.data.models || []).filter((m: any) => m.selected)
  } finally {
    providerModelsLoading.value = false
  }
}

async function fetchTools() {
  toolsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/mcp-tools`)
    tools.value = res.data.tools || []
  } finally {
    toolsLoading.value = false
  }
}

async function fetchResources() {
  resourcesLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/mcp-resources`)
    resources.value = res.data.resources || []
  } finally {
    resourcesLoading.value = false
  }
}

async function fetchPrompts() {
  promptsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/mcp-prompts`)
    prompts.value = res.data.prompts || []
  } finally {
    promptsLoading.value = false
  }
}

async function toggleModel(row: any) {
  const prev = row.enabled
  row.enabled = !prev
  try {
    const res = await api.put(`/keys/${keyId}/models/${row.id}`)
    row.enabled = res.data.enabled
    // 重新拉取以保持排序稳定（后端 ORDER BY id）
    await fetchModelTab()
  } catch (e: any) {
    row.enabled = prev
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function removeModel(row: any) {
  try {
    await api.delete(`/keys/${keyId}/models/${row.id}`)
    models.value = models.value.filter((m: any) => m.id !== row.id)
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function toggleProviderModel(row: any) {
  const prev = row.enabled
  row.enabled = !prev
  try {
    const res = await api.put(`/keys/${keyId}/provider-models/${row.id}`)
    row.enabled = res.data.enabled
    // 重新拉取以保持排序稳定（后端 ORDER BY id）
    await fetchProviderModels()
  } catch (e: any) {
    row.enabled = prev
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function removeProviderModel(row: any) {
  try {
    await api.delete(`/keys/${keyId}/provider-models/${row.id}`)
    providerModels.value = providerModels.value.filter((m: any) => m.id !== row.id)
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function toggleTool(row: any) {
  const previousValue = !row.selected
  try {
    if (row.selected) {
      await api.post(`/keys/${keyId}/mcp-tools/${row.id}`)
    } else {
      await api.delete(`/keys/${keyId}/mcp-tools/${row.id}`)
    }
  } catch (e: any) {
    row.selected = previousValue
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function toggleResource(row: any) {
  const previousValue = !row.selected
  try {
    if (row.selected) {
      await api.post(`/keys/${keyId}/mcp-resources/${row.id}`)
    } else {
      await api.delete(`/keys/${keyId}/mcp-resources/${row.id}`)
    }
  } catch (e: any) {
    row.selected = previousValue
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function togglePrompt(row: any) {
  const previousValue = !row.selected
  try {
    if (row.selected) {
      await api.post(`/keys/${keyId}/mcp-prompts/${row.id}`)
    } else {
      await api.delete(`/keys/${keyId}/mcp-prompts/${row.id}`)
    }
  } catch (e: any) {
    row.selected = previousValue
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function enableAllModels() {
  enablingModels.value = true
  try {
    await api.put(`/keys/${keyId}/models`)
    await fetchModelTab()
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    enablingModels.value = false
  }
}

async function disableAllModels() {
  disablingModels.value = true
  try {
    await api.delete(`/keys/${keyId}/models`)
    await fetchModelTab()
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    disablingModels.value = false
  }
}

async function enableAllProviderModels() {
  enablingProviderModels.value = true
  try {
    await api.put(`/keys/${keyId}/provider-models`)
    await fetchProviderModels()
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    enablingProviderModels.value = false
  }
}

async function disableAllProviderModels() {
  disablingProviderModels.value = true
  try {
    await api.delete(`/keys/${keyId}/provider-models`)
    await fetchProviderModels()
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    disablingProviderModels.value = false
  }
}

// 直通模型选择弹窗相关

async function showProviderModelDialog() {
  if (!directProvidersLoaded) {
    providerModelDialogLoading.value = true
    try {
      // 使用 key 的厂商过滤接口，只显示与密钥格式匹配的厂商
      const res = await api.get(`/keys/${keyId}/providers`)
      directProviders.value = (res.data.providers || []).sort((a: any, b: any) => a.name.localeCompare(b.name))
      directProvidersLoaded = true
    } catch { /* ignore */ }
    providerModelDialogLoading.value = false
  }
  // 确保已加载的模型列表是最新的
  await fetchProviderModels()
  directProviderForm.provider_id = null
  directModelSearch.value = ''
  directSelectedModelIDs.value = []
  directAvailableModels.value = []
  providerModelDialogVisible.value = true
}

async function onDirectProviderChange() {
  directModelSearch.value = ''
  directSelectedModelIDs.value = []
  if (!directProviderForm.provider_id) {
    directAvailableModels.value = []
    return
  }
  providerModelDialogLoading.value = true
  try {
    const res = await api.get(`/providers/${directProviderForm.provider_id}/models`)
    const models = (res.data.models || [])
    // 过滤：排除已加入白名单的模型（selected=true）
    const existingIds = new Set(providerModels.value.filter((m: any) => m.selected).map((m: any) => m.id))
    directAvailableModels.value = models
      .filter((m: any) => !existingIds.has(m.id))
      .sort((a: any, b: any) => a.model_id.localeCompare(b.model_id))
  } catch {
    directAvailableModels.value = []
  } finally {
    providerModelDialogLoading.value = false
  }
}

function onDirectModelSelectionChange(selection: any[]) {
  directSelectedModelIDs.value = selection.map((s: any) => s.id)
}

async function addSelectedProviderModels() {
  if (directSelectedModelIDs.value.length === 0) return
  addingProviderModels.value = true
  let successCount = 0
  let errorMsg = ''
  try {
    for (const pmid of directSelectedModelIDs.value) {
      try {
        await api.post(`/keys/${keyId}/provider-models/${pmid}`)
        // 如果之前被删除过，从 hidden 中移除
        successCount++
      } catch (e: any) {
        errorMsg = e.response?.data?.error || t('common.error')
        break
      }
    }
    if (successCount > 0) ElMessage.success(t('common.success'))
    if (errorMsg) ElMessage.error(errorMsg)
    providerModelDialogVisible.value = false
    await fetchProviderModels()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    addingProviderModels.value = false
  }
}

// 模型映射选择弹窗相关
const filteredAvailableModels = computed(() => {
  const search = modelSearch.value.toLowerCase()
  // 排除已加入白名单的模型（selected=true）
  const existingIds = new Set(models.value.filter((m: any) => m.selected).map((m: any) => m.id))
  return availableModelsForDialog.value.filter((m: any) => {
    if (existingIds.has(m.id)) return false
    if (!search) return true
    return m.name?.toLowerCase().includes(search)
  })
})

async function showModelDialog() {
  modelSearch.value = ''
  selectedModelIDs.value = []
  // 加载所有可选的虚拟模型（?all=true 返回未加入白名单的也列出来）
  try {
    const res = await api.get(`/keys/${keyId}/models`)
    availableModelsForDialog.value = res.data.models || []
    modelDialogVisible.value = true
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

function handleModelSelectionChange(selection: any[]) {
  selectedModelIDs.value = selection.map((s: any) => s.id)
}

async function addSelectedModels() {
  addingModels.value = true
  let successCount = 0
  let errorMsg = ''
  try {
    for (const mid of selectedModelIDs.value) {
      try {
        await api.post(`/keys/${keyId}/models/${mid}`)
        successCount++
      } catch (e: any) {
        errorMsg = e.response?.data?.error || t('common.error')
        break
      }
    }
    if (successCount > 0) ElMessage.success(`成功添加 ${successCount} 个模型`)
    if (errorMsg) ElMessage.error(errorMsg)
    modelDialogVisible.value = false
    await fetchModelTab()
  } finally {
    addingModels.value = false
  }
}

async function clearTools() {
  clearingTools.value = true
  try {
    await api.delete(`/keys/${keyId}/mcp-tools`)
    ElMessage.success(t('common.success'))
    tools.value.forEach(t => t.selected = false)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    clearingTools.value = false
  }
}

async function clearResources() {
  clearingResources.value = true
  try {
    await api.delete(`/keys/${keyId}/mcp-resources`)
    ElMessage.success(t('common.success'))
    resources.value.forEach(r => r.selected = false)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    clearingResources.value = false
  }
}

async function clearPrompts() {
  clearingPrompts.value = true
  try {
    await api.delete(`/keys/${keyId}/mcp-prompts`)
    ElMessage.success(t('common.success'))
    prompts.value.forEach(p => p.selected = false)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    clearingPrompts.value = false
  }
}

async function handleReset() {
  try {
    await ElMessageBox.confirm(
      t('key.resetConfirmMessage'),
      t('key.resetConfirmTitle'),
      { type: 'warning' }
    )
  } catch {
    return
  }

  resetting.value = true
  try {
    const res = await api.post(`/keys/${keyId}/reset`)
    key.value.key = res.data.key.key
    newKey.value = res.data.raw_key
    keyDialogVisible.value = true
    ElMessage.success(t('key.resetSuccess'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    resetting.value = false
  }
}

function copyKey() {
  copy(newKey.value)
}

function goBack() {
  router.push('/keys')
}

function isLongText(text: string): boolean {
  if (!text) return false
  const lines = text.split('\n')
  return lines.length > 5 || text.length > 300
}

function handleSortChange(key: string, { prop, order }: any) {
  if (prop && order) {
    setSortConfig(key, { prop, order })
  }
}
</script>

<style scoped>
.key-detail {
  padding: 20px;
}

.info-card {
  margin-top: 20px;
}

.info-card :deep(.el-descriptions__table) {
  width: 100%;
  table-layout: fixed;
}

.info-card :deep(.el-descriptions__cell) {
  width: 50%;
}

.actions {
  margin-top: 20px;
  display: flex;
  gap: 10px;
}

.tabs-card {
  margin-top: 20px;
}

.tab-actions {
  margin-bottom: 16px;
  text-align: right;
}

code {
  background-color: var(--el-fill-color-light);
  padding: 2px 6px;
  border-radius: 3px;
  font-family: monospace;
}

.description-cell {
  width: 100%;
}

.description-text {
  max-height: 7.5em;
  overflow: hidden;
  line-height: 1.5em;
  word-break: break-word;
  white-space: pre-wrap;
  position: relative;
}

.description-text.expanded {
  max-height: none;
}

.description-actions {
  margin-top: 4px;
  display: flex;
  gap: 8px;
}
</style>