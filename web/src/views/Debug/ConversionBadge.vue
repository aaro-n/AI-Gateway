<template>
  <el-popover
    placement="top"
    :width="320"
    trigger="hover"
    :disabled="!hasLoss"
  >
    <template #reference>
      <div class="conv-badge" :class="badgeClass">
        <span class="conv-path">
          <span class="proto-tag proto-entry">{{ entryAbbr }}</span>
          <span class="conv-arrow">→</span>
          <span class="proto-tag proto-upstream">{{ upstreamAbbr }}</span>
        </span>
        <span class="conv-label">{{ badgeText }}</span>
      </div>
    </template>
    <div class="loss-popover">
      <div class="loss-title">协议转换详情</div>
      <div class="loss-path">
        <strong>{{ props.entryProtocol }}</strong> → <strong>{{ props.upstreamProtocol }}</strong>
      </div>
      <div v-if="props.isDirect" class="loss-status loss-ok">
        ✅ 同协议直通，无功能损失
      </div>
      <div v-else>
        <div class="loss-status" :class="lossLevelClass">
          {{ lossSummary }}
        </div>
        <ul v-if="props.lostFeatures && props.lostFeatures.length > 0" class="loss-list">
          <li v-for="f in props.lostFeatures" :key="f">{{ f }}</li>
        </ul>
        <div v-else-if="props.convStatus && props.convStatus !== 'ok'" class="loss-raw">
          {{ props.convStatus }}
        </div>
      </div>
    </div>
  </el-popover>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  entryProtocol: string
  upstreamProtocol: string
  isDirect: boolean
  lostFeatures?: string[]
  convStatus?: string
}>()

const protoAbbr: Record<string, string> = {
  openai: 'OA',
  anthropic: 'AN',
  gemini: 'GM',
  deepseek: 'DS',
  openrouter: 'OR'
}

const entryAbbr = computed(() => protoAbbr[props.entryProtocol] || props.entryProtocol?.slice(0, 2).toUpperCase() || '??')
const upstreamAbbr = computed(() => protoAbbr[props.upstreamProtocol] || props.upstreamProtocol?.slice(0, 2).toUpperCase() || '??')

const lossCount = computed(() => props.lostFeatures?.length || 0)
const hasLoss = computed(() => !props.isDirect && (lossCount.value > 0 || (props.convStatus && props.convStatus !== 'ok')))

const badgeClass = computed(() => {
  if (props.isDirect) return 'badge-direct'
  if (lossCount.value === 0) return 'badge-direct'
  if (lossCount.value <= 2) return 'badge-mild'
  if (lossCount.value <= 5) return 'badge-moderate'
  return 'badge-severe'
})

const badgeText = computed(() => {
  if (props.isDirect) return '直通'
  if (lossCount.value === 0) {
    if (props.convStatus && props.convStatus !== 'ok') return props.convStatus
    return '直通'
  }
  return `${lossCount.value}项丢失`
})

const lossLevelClass = computed(() => {
  if (lossCount.value <= 2) return 'loss-mild'
  if (lossCount.value <= 5) return 'loss-moderate'
  return 'loss-severe'
})

const lossSummary = computed(() => {
  const count = lossCount.value
  if (count === 0) return '无功能损失'
  if (count <= 2) return `轻微降级 · ${count} 项功能丢失`
  if (count <= 5) return `中度降级 · ${count} 项功能丢失`
  return `严重降级 · ${count} 项功能丢失`
})
</script>

<style scoped>
.conv-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
  cursor: default;
  white-space: nowrap;
}

.badge-direct {
  background: #e6f7e6;
  color: #389e0d;
  border: 1px solid #b7eb8f;
}

.badge-mild {
  background: #e6f7ff;
  color: #096dd9;
  border: 1px solid #91d5ff;
}

.badge-moderate {
  background: #fff7e6;
  color: #d46b08;
  border: 1px solid #ffd591;
}

.badge-severe {
  background: #fff1f0;
  color: #cf1322;
  border: 1px solid #ffa39e;
}

.conv-path {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.proto-tag {
  display: inline-block;
  padding: 0 4px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 700;
}

.proto-entry {
  background: rgba(0, 0, 0, 0.06);
}

.proto-upstream {
  background: rgba(0, 0, 0, 0.06);
}

.conv-arrow {
  font-size: 10px;
  color: #999;
}

.conv-label {
  margin-left: 4px;
}

/* Popover 内容 */
.loss-popover {
  font-size: 13px;
}

.loss-title {
  font-weight: 600;
  margin-bottom: 8px;
  color: #303133;
}

.loss-path {
  margin-bottom: 8px;
  color: #606266;
}

.loss-status {
  margin-bottom: 8px;
  font-weight: 500;
}

.loss-ok {
  color: #389e0d;
}

.loss-mild {
  color: #096dd9;
}

.loss-moderate {
  color: #d46b08;
}

.loss-severe {
  color: #cf1322;
}

.loss-list {
  margin: 0;
  padding-left: 18px;
  color: #606266;
  font-size: 12px;
}

.loss-list li {
  margin-bottom: 2px;
}

.loss-raw {
  color: #909399;
  font-size: 12px;
}
</style>
