<template>
  <div v-if="visible" class="session-connecting-mask" aria-live="polite" :aria-busy="!done">
    <div class="session-connecting-card">
      <el-icon v-if="done" class="session-connecting-done" :size="36"><CircleCheckFilled /></el-icon>
      <el-icon v-else class="session-connecting-spin" :size="36"><Loading /></el-icon>
      <div class="session-connecting-text">{{ displayText }}</div>
      <div v-if="detail" class="session-connecting-detail">{{ detail }}</div>
      <div v-if="meta" class="session-connecting-meta">{{ meta }}</div>
      <el-button v-if="done" type="primary" class="session-connecting-close" @click="$emit('close')">
        {{ t('common.close') }}
      </el-button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleCheckFilled, Loading } from '@element-plus/icons-vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  text: { type: String, default: '' },
  /** Optional second line (e.g. download bytes / percent). */
  detail: { type: String, default: '' },
  /** Optional third line (e.g. speed · elapsed · ETA). */
  meta: { type: String, default: '' },
  /** Completion state: success icon + close button, no spinner. */
  done: { type: Boolean, default: false }
})

defineEmits(['close'])

const { t } = useI18n()
const displayText = computed(() => props.text || t('session.connecting'))
</script>

<style scoped>
.session-connecting-mask {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(2px);
}
.session-connecting-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 28px 36px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.18);
  color: #303133;
  max-width: min(480px, 90vw);
  text-align: center;
}
.session-connecting-spin {
  animation: session-connecting-rotate 1s linear infinite;
  color: var(--el-color-primary);
  margin-bottom: 4px;
}
.session-connecting-done {
  color: var(--el-color-success);
  margin-bottom: 4px;
}
.session-connecting-text {
  font-size: 14px;
  line-height: 1.4;
  word-break: break-all;
}
.session-connecting-detail {
  font-size: 13px;
  line-height: 1.4;
  color: #606266;
  font-variant-numeric: tabular-nums;
  word-break: break-all;
}
.session-connecting-meta {
  font-size: 12px;
  line-height: 1.4;
  color: #909399;
  font-variant-numeric: tabular-nums;
}
.session-connecting-close {
  margin-top: 8px;
}
@keyframes session-connecting-rotate {
  to { transform: rotate(360deg); }
}
</style>
