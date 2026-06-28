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
          <code>{{ key?.key }}</code>
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
            <el-table-column :label="t('common.status')" width="70" prop="selected" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" content="密钥已禁用" placement="top">
                  <el-switch v-model="row.selected" disabled />
                </el-tooltip>
                <el-switch v-else v-model="row.selected" @change="toggleProviderModel(row)" />
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
            <el-table-column :label="t('common.status')" width="70" prop="selected" sortable>
              <template #default="{ row }">
                <el-tooltip v-if="!key?.enabled" :content="t('key.keyDisabled')" placement="top">
                  <el-switch v-model="row.selected" disabled />
                </el-tooltip>
                <el-switch v-else v-model="row.selected" @change="toggleModel(row)" />
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

  <!-- 添加直通模型选择框（树形按厂商分组） -->
  <el-dialog v-model="providerModelDialogVisible" title="添加直通模型" width="700px">
    <el-input v-model="providerModelSearch" placeholder="搜索模型ID或厂商名称..." clearable style="margin-bottom: 12px;" />
    <div style="max-height: 400px; overflow-y: auto;">
      <el-tree
        ref="providerModelTreeRef"
        :data="providerModelTree"
        node-key="id"
        show-checkbox
        default-expand-all
        :filter-node-method="filterProviderModelNode"
        @check="onProviderModelTreeCheck"
      >
        <template #default="{ node, data }">
          <span v-if="data.isProvider" style="font-weight: 600;">
            {{ data.label }}
            <el-tag size="small" type="info" style="margin-left: 8px;">{{ data.modelCount }}</el-tag>
          </span>
          <span v-else style="display: inline-flex; align-items: center; gap: 12px;">
            <code style="min-width: 220px; font-size: 13px;">{{ data.label }}</code>
            <span style="color: var(--el-text-color-secondary); font-size: 12px;">{{ data.display_name }}</span>
          </span>
        </template>
      </el-tree>
      <el-empty v-if="providerModelTree.length === 0" description="无可用模型" />
    </div>
    <template #footer>
      <el-button @click="providerModelDialogVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="addSelectedProviderModels" :loading="addingProviderModels" :disabled="selectedProviderModelIDs.length === 0">
        {{ t('common.save') }} ({{ selectedProviderModelIDs.length }})
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
import { ref, computed, onMounted, watch } from 'vue'
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

const keyId = Number(route.params.id)
const key = ref<any>(null)
const loading = ref(false)
const resetting = ref(false)
const activeTab = ref('providers')
const keyDialogVisible = ref(false)
const newKey = ref('')

const models = ref<any[]>([])
const providers = ref<any[]>([])
const providerModels = ref<any[]>([])
const allProviderModels = ref<any[]>([])  // 所有可选的直通模型（含未选中）
const allAvailableModels = ref<any[]>([]) // 所有可选的映射模型（含未选中）
const tools = ref<any[]>([])
const resources = ref<any[]>([])
const prompts = ref<any[]>([])

const modelsLoading = ref(false)
const providersLoading = ref(false)
const providerModelsLoading = ref(false)
const toolsLoading = ref(false)
const resourcesLoading = ref(false)
const promptsLoading = ref(false)

// 跟踪已关联（曾经添加过）的模型 ID
const knownProviderModelIds = ref<Set<number>>(new Set())
const knownModelIds = ref<Set<number>>(new Set())

// localStorage 删除黑名单（持久化）
const LS_DELETED_PM = `deleted-pm-${keyId}`
const LS_DELETED_MODELS = `deleted-models-${keyId}`

function loadDeletedSet(lsKey: string): Set<number> {
  try {
    const raw = localStorage.getItem(lsKey)
    if (raw) return new Set(JSON.parse(raw))
  } catch { /* ignore */ }
  return new Set()
}

function saveDeletedSet(lsKey: string, set: Set<number>) {
  localStorage.setItem(lsKey, JSON.stringify([...set]))
}

// 运行时黑名单（从 localStorage 加载）
const deletedProviderModelIds = ref<Set<number>>(loadDeletedSet(LS_DELETED_PM))
const deletedModelIds = ref<Set<number>>(loadDeletedSet(LS_DELETED_MODELS))

const enablingModels = ref(false)
const enablingProviderModels = ref(false)
const disablingProviderModels = ref(false)
const disablingModels = ref(false)
const clearingTools = ref(false)
const clearingResources = ref(false)
const clearingPrompts = ref(false)

// 直通模型选择弹窗
const providerModelDialogVisible = ref(false)
const providerModelSearch = ref('')
const selectedProviderModelIDs = ref<number[]>([])
const addingProviderModels = ref(false)

// 模型映射选择弹窗
const modelDialogVisible = ref(false)
const modelSearch = ref('')
const selectedModelIDs = ref<number[]>([])
const addingModels = ref(false)

const modelsDefaultSort = getSortConfig('key-models', 'name')
const providersDefaultSort = getSortConfig('key-providers', 'name')
const toolsDefaultSort = getSortConfig('key-tools', 'name')
const resourcesDefaultSort = getSortConfig('key-resources', 'name')
const promptsDefaultSort = getSortConfig('key-prompts', 'name')

onMounted(() => {
  fetchKey()
})

watch(activeTab, (newTab) => {
  if (newTab === 'providers' && providerModels.value.length === 0) fetchProviderModels()
  if (newTab === 'models' && models.value.length === 0) fetchModels()
  if (newTab === 'tools' && tools.value.length === 0) fetchTools()
  if (newTab === 'resources' && resources.value.length === 0) fetchResources()
  if (newTab === 'prompts' && prompts.value.length === 0) fetchPrompts()
})

// 搜索时过滤树节点
watch(providerModelSearch, (val) => {
  providerModelTreeRef.value?.filter(val)
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

async function fetchModels() {
  modelsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/models`)
    allAvailableModels.value = res.data.models || []
    // 初始化 known set（首次加载时过滤掉黑名单中的 ID）
    if (knownModelIds.value.size === 0) {
      const ids = allAvailableModels.value
        .filter((m: any) => !deletedModelIds.value.has(m.id))
        .map((m: any) => m.id)
      knownModelIds.value = new Set(ids)
    }
    // 只显示已关联（在 known set 中）的模型
    models.value = allAvailableModels.value.filter((m: any) => knownModelIds.value.has(m.id))
  } finally {
    modelsLoading.value = false
  }
}

async function fetchProviderModels() {
  providerModelsLoading.value = true
  try {
    const res = await api.get(`/keys/${keyId}/provider-models`)
    allProviderModels.value = res.data.models || []
    // 初始化 known set（首次加载时过滤掉黑名单中的 ID）
    if (knownProviderModelIds.value.size === 0) {
      const ids = allProviderModels.value
        .filter((m: any) => !deletedProviderModelIds.value.has(m.id))
        .map((m: any) => m.id)
      knownProviderModelIds.value = new Set(ids)
    }
    // 只显示已关联（在 known set 中）的模型
    providerModels.value = allProviderModels.value.filter((m: any) => knownProviderModelIds.value.has(m.id))
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
  const previousValue = !row.selected
  try {
    if (row.selected) {
      await api.post(`/keys/${keyId}/models/${row.id}`)
      knownModelIds.value = new Set([...knownModelIds.value, row.id])
      // 重新添加时从删除黑名单移除
      if (deletedModelIds.value.has(row.id)) {
        const d = new Set(deletedModelIds.value)
        d.delete(row.id)
        deletedModelIds.value = d
        saveDeletedSet(LS_DELETED_MODELS, d)
      }
    } else {
      await api.delete(`/keys/${keyId}/models/${row.id}`)
      row.selected = false
    }
  } catch (e: any) {
    row.selected = previousValue
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function removeModel(row: any) {
  try {
    await api.delete(`/keys/${keyId}/models/${row.id}`)
    // 从 known set 移除
    const newKnown = new Set(knownModelIds.value)
    newKnown.delete(row.id)
    knownModelIds.value = newKnown
    // 加入删除黑名单并持久化
    const deleted = new Set(deletedModelIds.value)
    deleted.add(row.id)
    deletedModelIds.value = deleted
    saveDeletedSet(LS_DELETED_MODELS, deleted)
    // 从表格移除
    models.value = models.value.filter((m: any) => m.id !== row.id)
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function toggleProviderModel(row: any) {
  const previousValue = !row.selected
  try {
    if (row.selected) {
      await api.post(`/keys/${keyId}/provider-models/${row.id}`)
      knownProviderModelIds.value = new Set([...knownProviderModelIds.value, row.id])
      // 重新添加时从删除黑名单移除
      if (deletedProviderModelIds.value.has(row.id)) {
        const d = new Set(deletedProviderModelIds.value)
        d.delete(row.id)
        deletedProviderModelIds.value = d
        saveDeletedSet(LS_DELETED_PM, d)
      }
    } else {
      await api.delete(`/keys/${keyId}/provider-models/${row.id}`)
      row.selected = false
    }
  } catch (e: any) {
    row.selected = previousValue
    ElMessage.error(e.response?.data?.error || t('common.error'))
  }
}

async function removeProviderModel(row: any) {
  try {
    await api.delete(`/keys/${keyId}/provider-models/${row.id}`)
    // 从 known set 移除
    const newKnown = new Set(knownProviderModelIds.value)
    newKnown.delete(row.id)
    knownProviderModelIds.value = newKnown
    // 加入删除黑名单并持久化
    const deleted = new Set(deletedProviderModelIds.value)
    deleted.add(row.id)
    deletedProviderModelIds.value = deleted
    saveDeletedSet(LS_DELETED_PM, deleted)
    // 从表格移除
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
    // 将表格中未启用的模型全部 POST 启用
    for (const m of models.value) {
      if (!m.selected) {
        await api.post(`/keys/${keyId}/models/${m.id}`)
        m.selected = true
      }
    }
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
    // 将表格中已启用的模型全部 DELETE 禁用
    for (const m of models.value) {
      if (m.selected) {
        await api.delete(`/keys/${keyId}/models/${m.id}`)
        m.selected = false
      }
    }
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
    // 将表格中未启用的直通模型全部 POST 启用
    for (const m of providerModels.value) {
      if (!m.selected) {
        await api.post(`/keys/${keyId}/provider-models/${m.id}`)
        m.selected = true
      }
    }
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
    // 将表格中已启用的直通模型全部 DELETE 禁用
    for (const m of providerModels.value) {
      if (m.selected) {
        await api.delete(`/keys/${keyId}/provider-models/${m.id}`)
        m.selected = false
      }
    }
    ElMessage.success(t('common.success'))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t('common.error'))
  } finally {
    disablingProviderModels.value = false
  }
}

// 直通模型选择弹窗相关
const providerModelTreeRef = ref<any>(null)

// 构建 el-tree 数据：父节点=厂商，子节点=模型
const providerModelTree = computed(() => {
  const search = providerModelSearch.value.toLowerCase()
  // 排除所有已知关联模型 + 删除黑名单
  const available = allProviderModels.value.filter((m: any) => {
    if (knownProviderModelIds.value.has(m.id)) return false
    if (!search) return true
    return m.model_id?.toLowerCase().includes(search) ||
           m.provider_name?.toLowerCase().includes(search) ||
           m.display_name?.toLowerCase().includes(search)
  })

  // 按厂商分组构建树节点
  const groupMap = new Map<string, any[]>()
  for (const m of available) {
    const name = m.provider_name || '未知厂商'
    if (!groupMap.has(name)) groupMap.set(name, [])
    groupMap.get(name)!.push(m)
  }

  const tree: any[] = []
  for (const [providerName, models] of groupMap) {
    tree.push({
      id: `provider:${providerName}`,
      label: providerName,
      isProvider: true,
      modelCount: models.length,
      children: models.map((m: any) => ({
        id: m.id,
        label: m.model_id,
        display_name: m.display_name,
        isProvider: false,
      })),
    })
  }
  return tree
})

// 搜索过滤方法
function filterProviderModelNode(value: string, data: any): boolean {
  if (!value) return true
  const v = value.toLowerCase()
  if (data.isProvider) {
    return data.label.toLowerCase().includes(v)
  }
  return data.label.toLowerCase().includes(v) ||
         (data.display_name || '').toLowerCase().includes(v)
}

// 树节点勾选变化时更新选中 ID 列表
function onProviderModelTreeCheck(_checked: any, state: any) {
  selectedProviderModelIDs.value = state.checkedKeys.filter((k: any) => typeof k === 'number')
}

function showProviderModelDialog() {
  providerModelSearch.value = ''
  selectedProviderModelIDs.value = []
  fetchProviderModels().then(() => {
    providerModelDialogVisible.value = true
  })
}

async function addSelectedProviderModels() {
  addingProviderModels.value = true
  let successCount = 0
  let errorMsg = ''
  const newKnown = new Set(knownProviderModelIds.value)
  const newDeleted = new Set(deletedProviderModelIds.value)
  try {
    for (const pmid of selectedProviderModelIDs.value) {
      try {
        await api.post(`/keys/${keyId}/provider-models/${pmid}`)
        newKnown.add(pmid)
        newDeleted.delete(pmid)  // 重新添加时从黑名单移除
        successCount++
      } catch (e: any) {
        errorMsg = e.response?.data?.error || t('common.error')
        break
      }
    }
    if (successCount > 0) {
      knownProviderModelIds.value = newKnown
      deletedProviderModelIds.value = newDeleted
      saveDeletedSet(LS_DELETED_PM, newDeleted)
      ElMessage.success(`成功添加 ${successCount} 个模型`)
    }
    if (errorMsg) {
      ElMessage.error(errorMsg)
    }
    providerModelDialogVisible.value = false
    await fetchProviderModels()
  } finally {
    addingProviderModels.value = false
  }
}

// 模型映射选择弹窗相关
const filteredAvailableModels = computed(() => {
  const search = modelSearch.value.toLowerCase()
  // 排除所有已知关联模型（黑名单已在 fetch 时从 known set 排除）
  return allAvailableModels.value.filter((m: any) => {
    if (knownModelIds.value.has(m.id)) return false
    if (!search) return true
    return m.name?.toLowerCase().includes(search)
  })
})

async function showModelDialog() {
  modelSearch.value = ''
  selectedModelIDs.value = []
  // 加载所有可选的虚拟模型
  try {
    const res = await api.get(`/keys/${keyId}/models`)
    allAvailableModels.value = res.data.models || []
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
  const newKnown = new Set(knownModelIds.value)
  const newDeleted = new Set(deletedModelIds.value)
  try {
    for (const mid of selectedModelIDs.value) {
      try {
        await api.post(`/keys/${keyId}/models/${mid}`)
        newKnown.add(mid)
        newDeleted.delete(mid)  // 重新添加时从黑名单移除
        successCount++
      } catch (e: any) {
        errorMsg = e.response?.data?.error || t('common.error')
        break
      }
    }
    if (successCount > 0) {
      knownModelIds.value = newKnown
      deletedModelIds.value = newDeleted
      saveDeletedSet(LS_DELETED_MODELS, newDeleted)
      ElMessage.success(`成功添加 ${successCount} 个模型`)
    }
    if (errorMsg) {
      ElMessage.error(errorMsg)
    }
    modelDialogVisible.value = false
    await fetchModels()
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