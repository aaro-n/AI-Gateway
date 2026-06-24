<script setup lang="ts">
import { ref, onErrorCaptured } from 'vue'
import { ElAlert, ElButton, ElCollapse, ElCollapseItem } from 'element-plus'

const errors = ref<Array<{ message: string; stack?: string; time: Date }>>([])
const hasError = ref(false)

onErrorCaptured((err: Error, instance, info) => {
  errors.value.push({
    message: err.message || String(err),
    stack: err.stack,
    time: new Date(),
  })
  hasError.value = true
  console.error('[ErrorBoundary]', err, info)
  return false // 阻止向上传播
})

function clear() {
  errors.value = []
  hasError.value = false
}
</script>

<template>
  <div v-if="hasError" class="error-boundary">
    <el-alert
      title="页面出现异常"
      type="error"
      :description="`已捕获 ${errors.length} 个错误`"
      show-icon
      :closable="false"
    >
      <template #default>
        <el-button size="small" @click="clear" style="margin-top: 8px">
          清除并重试
        </el-button>
      </template>
    </el-alert>

    <el-collapse v-if="errors.length > 0" style="margin-top: 8px">
      <el-collapse-item
        v-for="(err, i) in errors"
        :key="i"
        :title="`${err.time.toLocaleTimeString()} — ${err.message.slice(0, 80)}`"
      >
        <pre class="error-stack">{{ err.stack || err.message }}</pre>
      </el-collapse-item>
    </el-collapse>
  </div>
  <slot v-else />
</template>

<style scoped>
.error-boundary {
  padding: 16px;
  max-width: 800px;
  margin: 0 auto;
}
.error-stack {
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 8px;
  border-radius: 4px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
