<template>
  <el-popover
    placement="top"
    :width="320"
    trigger="hover"
    :disabled="!hasInfo"
  >
    <template #reference>
      <div v-if="hasInfo" class="rt-conv-badge" :class="badgeClass">
        <span class="conv-path">
          <span class="proto-tag">{{ entryAbbr }}</span>
          <span class="conv-arrow">→</span>
          <span class="proto-tag">{{ upstreamAbbr }}</span>
        </span>
        <span class="conv-label">{{ badgeText }}</span>
      </div>
      <span v-else class="no-conv">-</span>
    </template>
    <div class="loss-popover">
      <div class="loss-title">协议转换详情</div>
      <div class="loss-path">
        <strong>{{ entryProto }}</strong> → <strong>{{ upstreamProto }}</strong>
      </div>
      <div v-if="isDirect" class="loss-status loss-ok">
        ✅ 同协议直通，无功能损失
      </div>
      <div v-else>
        <div class="loss-status" :class="lossLevelClass">
          {{ lossSummary }}
        </div>
        <ul v-if="lostFields.length > 0" class="loss-list">
          <li v-for="f in lostFields" :key="f">{{ f }}</li>
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
  source: string
  providerName: string
  callMethod: string
  convStatus?: string
  convDetail?: any
}>()

const protoAbbr: Record<string, string> = {
  openai: 'OA', anthropic: 'AN', gemini: 'GM', deepseek: 'DS', openrouter: 'OR'
}

// 解析 conv_detail 或 conv_status
const entryProto = computed(() => props.source || '-')
const upstreamProto = computed(() => {
  if (props.convDetail?.upstream_protocol) return props.convDetail.upstream_protocol
  // 从 providerName 推断，如果没有就用 source
  return entryProto.value
})

const isDirect = computed(() => {
  if (props.convDetail?.is_direct !== undefined) return props.convDetail.is_direct
  return props.callMethod === 'direct' && props.convStatus === 'ok'
})

const lostFields = computed(() => {
  if (props.convDetail?.lost_fields) return props.convDetail.lost_fields
  if (props.convStatus && props.convStatus !== 'ok') {
    return props.convStatus.split(',').filter(s => s.trim())
  }
  return []
})

const lossCount = computed(() => lostFields.value.length)

const hasInfo = computed(() => {
  if (props.callMethod === 'convert') return true
  if (props.convStatus && props.convStatus !== 'ok' && props.convStatus !== '') return true
  if (props.convDetail) return true
  return false
})

const entryAbbr = computed(() => protoAbbr[entryProto.value] || entryProto.value?.slice(0, 2).toUpperCase() || '??')
const upstreamAbbr = computed(() => protoAbbr[upstreamProto.value] || upstreamProto.value?.slice(0, 2).toUpperCase() || '??')

const badgeClass = computed(() => {
  if (isDirect.value) return 'badge-direct'
  if (lossCount.value === 0) return 'badge-direct'
  if (lossCount.value <= 2) return 'badge-mild'
  if (lossCount.value <= 5) return 'badge-moderate'
  return 'badge-severe'
})

const badgeText = computed(() => {
  if (isDirect.value) return '直通'
  if (lossCount.value === 0) return '直通'
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
.rt-conv-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 6px;
  border-radius: 10px;
  font-size: 11px;
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
  padding: 0 3px;
  border-radius: 2px;
  font-size: 9px;
  font-weight: 700;
  background: rgba(0, 0, 0, 0.06);
}

.conv-arrow { font-size: 9px; color: #999; }
.conv-label { margin-left: 2px; }

.no-conv {
  color: #c0c4cc;
}

.loss-popover {
  font-size: 13px;
}

.loss-title { font-weight: 600; margin-bottom: 8px; color: #303133; }
.loss-path { margin-bottom: 8px; color: #606266; }
.loss-status { margin-bottom: 8px; font-weight: 500; }
.loss-ok { color: #389e0d; }
.loss-mild { color: #096dd9; }
.loss-moderate { color: #d46b08; }
.loss-severe { color: #cf1322; }
.loss-list { margin: 0; padding-left: 18px; color: #606266; font-size: 12px; }
.loss-list li { margin-bottom: 2px; }
.loss-raw { color: #909399; font-size: 12px; }
</style>
