<template>
  <div class="protocol-compare">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('protocolCompare.title') }}</span>
        </div>
      </template>

      <p class="intro">{{ t('protocolCompare.intro') }}</p>

      <!-- 能力总览表 -->
      <el-card shadow="never" class="sub-card" v-loading="loading">
        <template #header>
          <span class="sub-title">{{ t('protocolCompare.capabilityOverview') }}</span>
        </template>
        <el-table :data="capabilityTable" border stripe style="width: 100%">
          <el-table-column prop="category" :label="t('protocolCompare.category')" width="120" />
          <el-table-column prop="capability" :label="t('protocolCompare.capability')" min-width="200" />
          <el-table-column v-for="p in protocols" :key="p.protocol" :label="p.label" width="140" align="center">
            <template #default="{ row }">
              <el-tag v-if="row[p.protocol]" type="success" size="small">{{ t('common.yes') }}</el-tag>
              <el-tag v-else type="info" size="small">{{ t('common.no') }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <!-- 两两对比 -->
      <el-card shadow="never" class="sub-card" style="margin-top: 20px">
        <template #header>
          <span class="sub-title">{{ t('protocolCompare.crossCompare') }}</span>
        </template>

        <el-row :gutter="20" style="margin-bottom: 20px">
          <el-col :span="10">
            <el-form-item :label="t('protocolCompare.fromProtocol')">
              <el-select v-model="fromProtocol" style="width: 100%" @change="onCompareChange">
                <el-option v-for="p in protocols" :key="p.protocol" :label="p.label" :value="p.protocol" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="4" style="text-align: center; line-height: 32px">
            <span style="font-size: 20px">→</span>
          </el-col>
          <el-col :span="10">
            <el-form-item :label="t('protocolCompare.toProtocol')">
              <el-select v-model="toProtocol" style="width: 100%" @change="onCompareChange">
                <el-option v-for="p in protocols" :key="p.protocol" :label="p.label" :value="p.protocol" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 对比结果 -->
        <div v-if="compareResult" class="compare-result">
          <el-alert
            :title="compareResult.summary"
            :type="compareResult.losses && compareResult.losses.length > 0 ? 'warning' : 'success'"
            :closable="false"
            show-icon
            style="margin-bottom: 16px"
          />

          <el-table v-if="compareResult.losses && compareResult.losses.length > 0" :data="compareResult.losses" border stripe>
            <el-table-column prop="capability_key" :label="t('protocolCompare.capability')" width="180" />
            <el-table-column :label="t('protocolCompare.lossLevel')" width="120" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.loss_level === 'full'" type="danger">{{ t('protocolCompare.fullLoss') }}</el-tag>
                <el-tag v-else-if="row.loss_level === 'partial'" type="warning">{{ t('protocolCompare.partialLoss') }}</el-tag>
                <el-tag v-else type="info">{{ row.loss_level }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="note" :label="t('protocolCompare.note')" min-width="400" />
          </el-table>

          <el-empty v-else :description="t('protocolCompare.noLoss')" />
        </div>
      </el-card>

      <!-- 单协议详情 -->
      <el-card shadow="never" class="sub-card" style="margin-top: 20px">
        <template #header>
          <span class="sub-title">{{ t('protocolCompare.detailView') }}</span>
        </template>
        <el-select v-model="selectedProtocol" style="width: 200px; margin-bottom: 16px" @change="onDetailChange">
          <el-option v-for="p in protocols" :key="p.protocol" :label="p.label" :value="p.protocol" />
        </el-select>

        <div v-if="selectedCaps" v-loading="detailLoading">
          <el-descriptions :title="selectedCaps.label + ' — ' + t('protocolCompare.capabilityList')" :column="1" border>
            <el-descriptions-item :label="t('protocolCompare.description')">
              {{ selectedCaps.description }}
            </el-descriptions-item>
          </el-descriptions>

          <el-table :data="groupedCapabilities" border stripe style="margin-top: 16px">
            <el-table-column prop="category" :label="t('protocolCompare.category')" width="120" />
            <el-table-column prop="label" :label="t('protocolCompare.capability')" width="220" />
            <el-table-column prop="description" :label="t('protocolCompare.description')" />
          </el-table>
        </div>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@/api'

const { t } = useI18n()

interface Capability {
  key: string
  label: string
  description: string
  category: string
}

interface ProtocolCaps {
  protocol: string
  label: string
  description: string
  capabilities: Capability[]
}

interface ConversionLoss {
  capability_key: string
  loss_level: string
  note: string
}

interface CompareResult {
  from_protocol: string
  from_label: string
  to_protocol: string
  to_label: string
  losses: ConversionLoss[]
  summary: string
}

const loading = ref(false)
const detailLoading = ref(false)
const protocols = ref<ProtocolCaps[]>([])
const fromProtocol = ref('openai')
const toProtocol = ref('anthropic')
const compareResult = ref<CompareResult | null>(null)
const selectedProtocol = ref('openai')
const selectedCaps = ref<ProtocolCaps | null>(null)

const categoryLabels: Record<string, string> = {
  core: '核心能力',
  output: '输出控制',
  input: '输入控制',
  advanced: '高级功能'
}

// 构建能力对比表数据
const capabilityTable = computed(() => {
  if (protocols.value.length === 0) return []
  // 收集所有能力（去重）
  const allCaps = new Map<string, { category: string; label: string; description: string }>()
  const protoKeys = protocols.value.map(p => p.protocol)
  
  for (const p of protocols.value) {
    for (const cap of p.capabilities) {
      if (!allCaps.has(cap.key)) {
        allCaps.set(cap.key, { category: categoryLabels[cap.category] || cap.category, label: cap.label, description: cap.description })
      }
    }
  }

  return Array.from(allCaps.entries()).map(([key, info]) => {
    const row: any = { capability: info.label, category: info.category }
    for (const p of protocols.value) {
      row[p.protocol] = p.capabilities.some(c => c.key === key)
    }
    return row
  })
})

const groupedCapabilities = computed(() => {
  if (!selectedCaps.value) return []
  return selectedCaps.value.capabilities.map(c => ({
    ...c,
    category: categoryLabels[c.category] || c.category
  }))
})

onMounted(async () => {
  await fetchProtocols()
  await fetchCompare()
  await fetchDetail()
})

async function fetchProtocols() {
  loading.value = true
  try {
    const { data } = await api.get('/protocols/compare')
    protocols.value = data.protocols || []
    if (protocols.value.length >= 2) {
      fromProtocol.value = protocols.value[0].protocol
      toProtocol.value = protocols.value[1].protocol
      selectedProtocol.value = protocols.value[0].protocol
    }
  } finally {
    loading.value = false
  }
}

async function fetchCompare() {
  try {
    const { data } = await api.get(`/protocols/compare-between/${fromProtocol.value}/${toProtocol.value}`)
    compareResult.value = data
  } catch {
    // API 失败时显示友好提示
    compareResult.value = {
      from_protocol: fromProtocol.value,
      from_label: protocols.value.find(p => p.protocol === fromProtocol.value)?.label || fromProtocol.value,
      to_protocol: toProtocol.value,
      to_label: protocols.value.find(p => p.protocol === toProtocol.value)?.label || toProtocol.value,
      losses: [],
      summary: t('protocolCompare.errorHint')
    }
  }
}

async function fetchDetail() {
  detailLoading.value = true
  try {
    const { data } = await api.get(`/protocols/compare/${selectedProtocol.value}`)
    selectedCaps.value = data
  } finally {
    detailLoading.value = false
  }
}

function onCompareChange() {
  if (fromProtocol.value === toProtocol.value) {
    // 相同协议：直接显示无损失
    const fromLabel = protocols.value.find(p => p.protocol === fromProtocol.value)?.label || fromProtocol.value
    compareResult.value = {
      from_protocol: fromProtocol.value,
      from_label: fromLabel,
      to_protocol: toProtocol.value,
      to_label: fromLabel,
      losses: [],
      summary: t('protocolCompare.noLoss')
    }
    return
  }
  fetchCompare()
}

function onDetailChange() {
  fetchDetail()
}
</script>

<style scoped>
.protocol-compare {
  max-width: 1200px;
}

.intro {
  color: var(--el-text-color-secondary);
  margin-bottom: 20px;
  line-height: 1.6;
}

.sub-card {
  margin-top: 0;
}

.sub-title {
  font-weight: 600;
  font-size: 15px;
}

.compare-result {
  margin-top: 16px;
}
</style>
