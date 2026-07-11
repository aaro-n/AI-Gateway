<template>
  <div class="debug-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('debug.title') }}</span>
        </div>
      </template>

      <p class="intro">{{ t('debug.intro') }}</p>

      <!-- ========== 区域1: 测试模型供应商 ========== -->
      <el-card shadow="never" class="sub-card">
        <template #header>
          <div class="sub-header-row">
            <span class="sub-title">{{ t('debug.providerTest') }}</span>
            <el-button text type="primary" @click="providerCollapsed = !providerCollapsed">
              {{ providerCollapsed ? t('common.expand') : t('common.collapse') }}
            </el-button>
          </div>
        </template>

        <el-collapse-transition>
          <div v-show="!providerCollapsed">
            <el-row :gutter="16" style="margin-bottom: 16px">
              <el-col :span="8">
                <el-select
                  v-model="selectedProviderId"
                  :placeholder="t('debug.selectProvider')"
                  clearable
                  style="width: 100%"
                  @change="onProviderChange"
                >
                  <el-option
                    v-for="p in providers"
                    :key="p.id"
                    :label="p.name"
                    :value="p.id"
                  />
                </el-select>
              </el-col>
              <el-col :span="8">
                <el-select
                  v-model="selectedTestModel"
                  :placeholder="selectedProviderId ? '选择模型（默认自动选取）' : '请先选择厂商'"
                  clearable
                  :disabled="!selectedProviderId || availableTestModels.length === 0"
                  style="width: 100%"
                >
                  <el-option
                    v-for="m in availableTestModels"
                    :key="m"
                    :label="m"
                    :value="m"
                  />
                </el-select>
              </el-col>
              <el-col :span="4">
                <el-button
                  type="primary"
                  :loading="providerTesting"
                  @click="testProviders"
                >
                  {{ t('debug.test') }}
                </el-button>
              </el-col>

            </el-row>

            <!-- Provider 测试结果列表 -->
            <div v-if="providerResults.length > 0" class="results-list">
              <div v-for="(result, idx) in providerResults" :key="idx" class="result-card">
                <!-- 结果头 -->
                <div class="result-head" :class="result.success ? 'success' : 'error'">
                  <span class="result-name">{{ result.provider_name }} / {{ result.protocol }}</span>
                  <el-tag :type="result.success ? 'success' : 'danger'" size="small">
                    {{ result.success ? t('debug.pass') : t('debug.fail') }}
                    {{ result.latency_ms }}ms
                  </el-tag>
                </div>

                <!-- 请求/响应日志 -->
                <div class="request-log-section">
                  <div class="section-label">📤 {{ t('debug.requestLogs') }}</div>
                  <div v-if="result.requestDump" class="dump-block">
                    <pre class="dump-content">{{ result.requestDump }}</pre>
                  </div>
                  <div v-else class="dump-empty">{{ t('debug.noData') }}</div>

                  <div class="section-label" style="margin-top: 12px">📥 {{ t('debug.responseLogs') }}</div>
                  <div v-if="result.responseBody" class="dump-block">
                    <pre class="dump-content">{{ result.responseBody }}</pre>
                  </div>
                  <div v-else class="dump-empty">{{ t('debug.noData') }}</div>
                </div>
              </div>
            </div>
          </div>
        </el-collapse-transition>
      </el-card>

      <!-- ========== 区域2: 测试API密钥 ========== -->
      <el-card shadow="never" class="sub-card">
        <template #header>
          <div class="sub-header-row">
            <span class="sub-title">{{ t('debug.keyTest') }}</span>
            <el-button text type="primary" @click="keyCollapsed = !keyCollapsed">
              {{ keyCollapsed ? t('common.expand') : t('common.collapse') }}
            </el-button>
          </div>
        </template>

        <el-collapse-transition>
          <div v-show="!keyCollapsed">
            <el-row :gutter="16" style="margin-bottom: 16px">
              <el-col :span="6">
                <el-select
                  v-model="selectedKeyId"
                  :placeholder="t('debug.selectKey')"
                  style="width: 100%"
                  @change="onKeyChange"
                >
                  <el-option
                    v-for="k in keys"
                    :key="k.id"
                    :label="k.name"
                    :value="k.id"
                  />
                </el-select>
              </el-col>
              <el-col :span="6">
                <el-select
                  v-model="keyModelInput"
                  :placeholder="selectedKeyId ? '选择模型（默认 gpt-4o）' : '请先选择密钥'"
                  clearable
                  filterable
                  :disabled="!selectedKeyId || keyAvailableModels.length === 0"
                  style="width: 100%"
                >
                  <el-option
                    v-for="m in keyAvailableModels"
                    :key="m"
                    :label="m"
                    :value="m"
                  />
                </el-select>
              </el-col>
              <el-col :span="4">
                <el-button
                  type="primary"
                  :loading="keyTesting"
                  :disabled="!selectedKeyId"
                  @click="testKey"
                >
                  {{ t('debug.test') }}
                </el-button>
              </el-col>
            </el-row>

            <div v-if="selectedKeyInfo" class="key-info-bar">
              <el-tag type="info" size="small">{{ selectedKeyInfo.protocol }}</el-tag>
              <span style="margin: 0 8px; color: #909399">{{ selectedKeyInfo.access_mode }}</span>
            </div>

            <!-- Key 测试结果 -->
            <div v-if="keyResult" class="result-card">
              <div class="result-head" :class="keyResult.success ? 'success' : 'error'">
                <span class="result-name">
                  {{ keyResult.key_name }} / {{ keyResult.protocol }} / {{ keyResult.model }}
                </span>
                <span class="result-meta">
                  <el-tag :type="keyResult.success ? 'success' : 'danger'" size="small">
                    {{ keyResult.success ? `HTTP ${keyResult.http_status}` : t('debug.fail') }}
                    {{ keyResult.latency_ms }}ms
                  </el-tag>
                  <conversion-badge
                    v-if="keyResult.success"
                    :entry-protocol="keyResult.entry_protocol"
                    :upstream-protocol="keyResult.upstream_proto || keyResult.protocol"
                    :is-direct="keyResult.is_direct"
                    :lost-features="keyResult.lost_features"
                    :conv-status="keyResult.conv_status"
                    style="margin-left: 8px"
                  />
                </span>
              </div>

              <!-- 运行日志：仅显示本次请求/响应的 curl dump -->
              <div class="request-log-section">
                <div class="section-label">📋 {{ t('debug.runLog') }}</div>
                <div v-if="keyResult.logs && keyResult.logs.length > 0" class="log-container key-test-log">
                  <div class="log-lines">
                    <div
                      v-for="(log, li) in keyResult.logs"
                      :key="li"
                      class="log-line"
                      :class="'log-' + log.level"
                    >
                      <span class="log-time">{{ log.timestamp }}</span>
                      <span class="log-level-tag">{{ log.level.toUpperCase() }}</span>
                      <span class="log-msg">{{ log.message }}</span>
                      <pre v-if="log.detail" class="log-detail">{{ log.detail }}</pre>
                    </div>
                  </div>
                </div>
                <div v-else class="dump-empty">{{ t('debug.noData') }}</div>
              </div>
            </div>
          </div>
        </el-collapse-transition>
      </el-card>

      <!-- ========== 区域3: 服务运行日志 ========== -->
      <el-card shadow="never" class="sub-card">
        <template #header>
          <div class="sub-header-row">
            <span class="sub-title">🖥️ {{ t('debug.serverLogs') }}</span>
            <div class="sub-header-actions">
              <el-tag :type="isAtBottom ? 'success' : 'info'" size="small" style="margin-right: 8px;">
                {{ isAtBottom ? '自动跟随' : '查看历史' }}
              </el-tag>
              <el-button text type="primary" :loading="logsLoading" @click="fetchServerLogs(true)">
                <el-icon><Refresh /></el-icon>
                {{ t('debug.refresh') }}
              </el-button>
            </div>
          </div>
        </template>

        <div v-if="serverLogs.length > 0" class="log-container server-log-container" ref="serverLogContainer" @scroll="onLogScroll">
          <div class="log-lines">
            <div
              v-for="(log, li) in serverLogs"
              :key="li"
              class="log-line"
              :class="'log-' + log.level.toLowerCase()"
            >
              <span class="log-time">{{ log.timestamp }}</span>
              <span class="log-level-tag">{{ log.level.toUpperCase() }}</span>
              <span class="log-msg">{{ log.message }}</span>
              <span v-if="log.trace_id" class="log-trace">trace:{{ log.trace_id }}</span>
            </div>
          </div>
        </div>
        <el-empty v-else :description="t('debug.noServerLogs')">
          <el-button type="primary" size="small" @click="fetchServerLogs(true)">
            {{ t('debug.refresh') }}
          </el-button>
        </el-empty>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'
import api from '@/api'
import ConversionBadge from './ConversionBadge.vue'

const { t } = useI18n()

// ── 区域1: Provider 测试 ──
const providerCollapsed = ref(true)
const providers = ref<any[]>([])
const selectedProviderId = ref<number | undefined>(undefined)
const selectedTestModel = ref('')
const availableTestModels = ref<string[]>([])
const providerTesting = ref(false)
const providerResults = ref<any[]>([])

// ── 区域2: Key 测试 ──
const keyCollapsed = ref(true)
const keys = ref<any[]>([])
const selectedKeyId = ref<number | undefined>(undefined)
const selectedKeyInfo = ref<any>(null)
const keyModelInput = ref('')
const keyAvailableModels = ref<string[]>([])
const keyTesting = ref(false)
const keyResult = ref<any>(null)

// ── 区域3: 服务运行日志 ──
const logsLoading = ref(false)
const serverLogs = ref<any[]>([])
const isAtBottom = ref(true)        // 是否在底部（自动跟随 vs 查看历史）
let pollTimer: ReturnType<typeof setInterval> | null = null
const serverLogContainer = ref<HTMLElement>()

// ── 生命周期 ──
onMounted(async () => {
  await Promise.all([fetchProviders(), fetchKeys()])
  fetchServerLogs(true)
  startPolling()
})

onUnmounted(() => {
  stopPolling()
})

async function fetchProviders() {
  try {
    const res = await api.get('/providers')
    providers.value = res.data.providers || []
  } catch (e) {
    console.error('Failed to load providers', e)
  }
}

async function fetchKeys() {
  try {
    const res = await api.get('/keys')
    keys.value = res.data.keys || []
  } catch (e) {
    console.error('Failed to load keys', e)
  }
}

// ── 服务运行日志（首次全量50条，后续增量轮询）──
async function fetchServerLogs(reset: boolean) {
  logsLoading.value = true
  try {
    const el = serverLogContainer.value
    // ★ 先记下当前是否在底部（日志追加后 scrollHeight 会变，必须提前检测）
    let wasAtBottom = true
    if (el) {
      wasAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    }

    if (reset) {
      const res = await api.get('/debug/server-logs')
      serverLogs.value = res.data.logs || []
    } else {
      const lastTs = serverLogs.value.length > 0
        ? serverLogs.value[serverLogs.value.length - 1].timestamp
        : ''
      const res = await api.get('/debug/server-logs', { params: { since: lastTs } })
      const newLogs = res.data.logs || []
      if (newLogs.length > 0) {
        serverLogs.value = [...serverLogs.value, ...newLogs]
        if (serverLogs.value.length > 200) {
          serverLogs.value = serverLogs.value.slice(-200)
        }
      }
    }

    if (reset) {
      // 首次加载：等 DOM 完全渲染后再滚到底
      await nextTick()
      requestAnimationFrame(() => {
        const container = serverLogContainer.value
        if (container) {
          container.scrollTop = container.scrollHeight
          isAtBottom.value = true
        }
      })
    } else if (wasAtBottom) {
      await nextTick()
      const container = serverLogContainer.value
      if (container) {
        container.scrollTop = container.scrollHeight
      }
    }
  } catch (e) {
    console.error('Failed to load server logs', e)
  } finally {
    logsLoading.value = false
  }
}

// 用户滚动时实时更新 isAtBottom
function onLogScroll() {
  const el = serverLogContainer.value
  if (!el) return
  isAtBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

function startPolling() {
  stopPolling()
  // ★ 始终轮询（不停止），只根据 isAtBottom 决定是否滚到最新
  pollTimer = setInterval(() => {
    fetchServerLogs(false)
  }, 5000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// ── Provider 测试 ──
function onProviderChange() {
  providerResults.value = []
  selectedTestModel.value = ''
  availableTestModels.value = []
  
  if (selectedProviderId.value) {
    fetchProviderModels(selectedProviderId.value)
  }
}

async function fetchProviderModels(providerId: number) {
  try {
    const res = await api.get(`/providers/${providerId}`)
    const models = res.data.provider?.models || []
    availableTestModels.value = models.map((m: any) => m.model_id || m.ModelID || '').filter((id: string) => id !== '')
  } catch (e) {
    console.error('Failed to fetch provider models', e)
  }
}

async function testProviders() {
  providerTesting.value = true
  providerResults.value = []
  try {
    if (providers.value.length === 0) {
      providerResults.value = [{
        provider_name: '提示',
        protocol: '-',
        success: false,
        latency_ms: 0,
        requestDump: '',
        responseBody: '没有可用的模型厂商，请先到「模型厂商」页面添加 Provider。'
      }]
      return
    }

    const body: any = {}
    if (selectedProviderId.value) {
      body.provider_id = selectedProviderId.value
    }
    if (selectedTestModel.value) {
      body.model = selectedTestModel.value
    }
    const res = await api.post('/debug/test-providers', body)
    const rawResults = res.data.results || []

    // 从 logs 中提取请求体和响应体
    providerResults.value = rawResults.map((r: any) => {
      let requestDump = ''
      let responseBody = ''
      if (r.logs) {
        for (const log of r.logs) {
          if (log.message === '发送测试请求...' && log.detail) {
            requestDump = log.detail
          }
          if (log.message === '响应内容') {
            responseBody = log.detail !== undefined ? log.detail : ''
          }
          if (log.message === '错误详情' && log.detail && !responseBody) {
            responseBody = log.detail
          }
        }
      }

      return {
        provider_id: r.provider_id,
        provider_name: r.provider_name,
        protocol: r.protocol,
        test_model: r.test_model,
        success: r.success,
        latency_ms: r.latency_ms,
        input_tokens: r.input_tokens,
        output_tokens: r.output_tokens,
        requestDump,
        responseBody
      }
    })
  } catch (e: any) {
    console.error('Provider test failed', e)
  } finally {
    providerTesting.value = false
  }
}

// ── Key 测试 ──
function onKeyChange(val: number | undefined) {
  keyResult.value = null
  selectedKeyInfo.value = null
  keyModelInput.value = ''
  keyAvailableModels.value = []
  if (val) {
    const k = keys.value.find((k: any) => k.id === val)
    if (k) {
      selectedKeyInfo.value = {
        protocol: k.format || k.formats?.[Object.keys(k.formats || {})[0]] || '-',
        access_mode: k.access_mode
      }
      fetchKeyBoundModels(val)
    }
  }
}

async function fetchKeyBoundModels(keyId: number) {
  try {
    const models: string[] = []
    // 映射模式：虚拟模型名
    const mappingRes = await api.get(`/keys/${keyId}/models`).catch(() => null)
    if (mappingRes?.data?.models) {
      for (const m of mappingRes.data.models) {
        models.push(m.name || m.model_name || m.model_id || '')
      }
    }
    // 直通模式：provider model IDs
    const directRes = await api.get(`/keys/${keyId}/provider-models`).catch(() => null)
    if (directRes?.data?.models) {
      for (const m of directRes.data.models) {
        models.push(m.model_id || m.ModelID || '')
      }
    }
    keyAvailableModels.value = [...new Set(models)].filter(Boolean)
  } catch (e) {
    console.error('Failed to fetch key bound models', e)
  }
}

async function testKey() {
  if (!selectedKeyId.value) return
  keyTesting.value = true
  keyResult.value = null
  try {
    const body: any = { key_id: selectedKeyId.value }
    if (keyModelInput.value) {
      body.model = keyModelInput.value.trim()
    }
    const res = await api.post('/debug/test-key', body)
    keyResult.value = res.data
  } catch (e: any) {
    console.error('Key test failed', e)
  } finally {
    keyTesting.value = false
  }
}
</script>

<style scoped>
.debug-page {
  padding: 0;
}

.intro {
  color: #909399;
  margin-bottom: 20px;
}

.sub-card {
  margin-bottom: 20px;
}

.sub-header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sub-header-actions {
  display: flex;
  align-items: center;
}

.sub-title {
  font-weight: 600;
  font-size: 15px;
}

/* ── 测试结果卡片 ── */
.results-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.result-card {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  overflow: hidden;
}

.result-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  font-weight: 600;
  font-size: 13px;
  border-bottom: 1px solid #ebeef5;
}

.result-head.success {
  background: #f0fdf4;
  color: #16a34a;
}

.result-head.error {
  background: #fef2f2;
  color: #dc2626;
}

.result-name {
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace;
}

.result-meta {
  display: flex;
  align-items: center;
}

/* 请求/响应日志区域 */
.request-log-section {
  padding: 10px 14px;
}

.section-label {
  font-size: 12px;
  font-weight: 600;
  color: #606266;
  margin-bottom: 6px;
}

.dump-block {
  background: #1e1e1e;
  border-radius: 4px;
  padding: 8px 12px;
  max-height: 300px;
  overflow-y: auto;
}

.dump-content {
  margin: 0;
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace;
  font-size: 12px;
  color: #c9d1d9;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
}

.dump-empty {
  color: #909399;
  font-size: 12px;
  padding-left: 4px;
}

/* ── Key 测试日志容器 ── */
.key-test-log {
  max-height: 500px;
}

/* ── 服务运行日志容器 ── */
.log-container {
  overflow-y: auto;
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace;
  font-size: 13px;
  background: #1e1e1e;
  border-radius: 6px;
  padding: 0;
}

.server-log-container {
  max-height: 600px;
}

/* ── 日志行 ── */
.log-lines {
  padding: 4px 0;
}

.log-line {
  padding: 3px 12px;
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 8px;
  line-height: 1.5;
  border-left: 3px solid transparent;
}

.log-line.log-info {
  color: #c9d1d9;
  border-left-color: #58a6ff;
}

.log-line.log-success {
  color: #7ee787;
  border-left-color: #3fb950;
}

.log-line.log-error {
  color: #f87171;
  border-left-color: #f85149;
  background: rgba(248, 81, 73, 0.05);
}

.log-line.log-warn {
  color: #d29922;
  border-left-color: #d29922;
  background: rgba(210, 153, 34, 0.05);
}

.log-time {
  color: #6e7681;
  flex-shrink: 0;
  font-size: 12px;
}

.log-level-tag {
  flex-shrink: 0;
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 3px;
  font-weight: 700;
  opacity: 0.7;
}

.log-msg {
  word-break: break-all;
}

.log-detail {
  width: 100%;
  margin: 4px 0 0 0;
  padding: 6px 10px;
  background: #0d1117;
  border-radius: 4px;
  color: #8b949e;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}

.log-trace {
  color: #6e7681;
  font-size: 11px;
  flex-shrink: 0;
  margin-left: auto;
}

/* ── 密钥信息栏 ── */
.key-info-bar {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
}
</style>
