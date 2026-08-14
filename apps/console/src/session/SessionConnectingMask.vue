<template>
  <div v-if="visible" class="session-connecting-mask" aria-live="polite" aria-busy="true">
    <div class="session-connecting-card">
      <el-icon class="session-connecting-spin" :size="36"><Loading /></el-icon>
      <div class="session-connecting-text">{{ displayText }}</div>
      <div v-if="detail" class="session-connecting-detail">{{ detail }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loading } from '@element-plus/icons-vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  text: { type: String, default: '' },
  /** Optional second line (e.g. download bytes / percent). */
  detail: { type: String, default: '' }
})

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
  max-width: min(420px, 90vw);
  text-align: center;
}
.session-connecting-spin {
  animation: session-connecting-rotate 1s linear infinite;
  color: var(--el-color-primary);
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
}
@keyframes session-connecting-rotate {
  to { transform: rotate(360deg); }
}
</style>
