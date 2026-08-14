<template>
  <el-dialog
    v-model="visible"
    :title="t('filetransfer.uploadTasks')"
    width="80%"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
    :destroy-on-close="false"
  >
    <div class="upload-summary">
      <span>{{ t('filetransfer.totalFiles', { n: items.length }) }}</span>
      <span>{{ t('filetransfer.pending', { n: pendingCount }) }}</span>
      <span>{{ t('filetransfer.running', { n: runningCount }) }}</span>
      <span>{{ t('filetransfer.done', { n: doneCount }) }}</span>
      <span>{{ t('filetransfer.error', { n: errorCount }) }}</span>
      <span>{{ t('filetransfer.uploadedOf', { uploaded: formatSize(uploadedBytes), total: formatSize(totalBytes) }) }}</span>
      <span v-if="uploading">{{ t('filetransfer.overallSpeed', { speed: formatSpeed(overallSpeed) }) }}</span>
      <span v-if="uploading">{{ t('filetransfer.overallEta', { eta: overallEta }) }}</span>
    </div>
    <el-table :data="items" height="420" size="small" stripe>
      <el-table-column :label="t('filetransfer.file')" min-width="200" prop="relativePath" show-overflow-tooltip />
      <el-table-column :label="t('filetransfer.uploadedSize')" width="170">
        <template #default="{ row }">
          {{ formatSize(row.loaded || 0) }} / {{ formatSize(row.size) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.status')" width="90">
        <template #default="{ row }">
          <el-tag size="small" :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('filetransfer.progress')" width="150">
        <template #default="{ row }">
          <el-progress
            :percentage="progressOf(row)"
            :stroke-width="10"
            :status="row.status === 'error' ? 'exception' : (row.status === 'done' ? 'success' : undefined)"
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('filetransfer.speed')" width="120">
        <template #default="{ row }">{{ rowSpeed(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('filetransfer.remaining')" width="90">
        <template #default="{ row }">{{ rowEta(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('filetransfer.elapsed')" width="90">
        <template #default="{ row }">{{ rowElapsed(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('filetransfer.note')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.error || '-' }}</template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button
        v-if="errorCount || pausedCount"
        :disabled="uploading"
        @click="emit('resume')"
      >
        {{ t('filetransfer.resumeFailed') }}
      </el-button>
      <el-button
        v-if="uploading || runningCount"
        type="danger"
        plain
        @click="emit('cancel')"
      >
        {{ t('filetransfer.cancelAll') }}
      </el-button>
      <el-button :disabled="uploading" type="primary" @click="close">
        <IconX :size="16" stroke="1.75" />
        {{ t('common.close') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { IconX } from '@tabler/icons-vue'
import { formatDuration, formatSize, formatSpeed } from './transferFormat'

const { t } = useI18n()

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  items: { type: Array, default: () => [] },
  uploading: { type: Boolean, default: false },
  overallSpeed: { type: Number, default: 0 },
  nowTick: { type: Number, default: 0 }
})
const emit = defineEmits(['update:modelValue', 'resume', 'cancel'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const countOf = (...states) => props.items.filter((i) => states.includes(i.status)).length
const pendingCount = computed(() => countOf('pending'))
const runningCount = computed(() => countOf('uploading', 'retrying'))
const doneCount = computed(() => countOf('done'))
const errorCount = computed(() => countOf('error'))
const pausedCount = computed(() => countOf('paused'))
const totalBytes = computed(() => props.items.reduce((s, i) => s + (i.size || 0), 0))
const uploadedBytes = computed(() => props.items.reduce((s, i) => s + (i.loaded || 0), 0))
const overallEta = computed(() => {
  const remain = Math.max(0, totalBytes.value - uploadedBytes.value)
  if (!remain) return '0s'
  if (props.overallSpeed <= 0) return '-'
  return formatDuration((remain / props.overallSpeed) * 1000)
})

function progressOf(row) {
  if (!row.size) return row.status === 'done' ? 100 : (row.status === 'uploading' ? 100 : 0)
  return Math.min(100, Math.round(((row.loaded || 0) / row.size) * 100))
}

function statusLabel(s) {
  return ({
    pending: t('filetransfer.statusPending'),
    uploading: t('filetransfer.statusUploading'),
    retrying: t('filetransfer.statusRetrying'),
    paused: t('filetransfer.statusPaused'),
    done: t('filetransfer.statusDone'),
    error: t('filetransfer.statusError')
  })[s] || s
}

function statusType(s) {
  return ({
    pending: 'info',
    uploading: '',
    retrying: 'warning',
    paused: 'warning',
    done: 'success',
    error: 'danger'
  })[s] || 'info'
}

function rowSpeed(row) {
  if (row.status === 'uploading' || row.status === 'retrying') return formatSpeed(row.speed || 0)
  // Finished rows show the average of what this run actually pushed (resumed bytes excluded).
  const ms = (row.finishedAt || 0) - (row.startedAt || 0)
  if (ms > 0 && row.runBytes > 0) return t('filetransfer.avgSpeed', { speed: formatSpeed((row.runBytes * 1000) / ms) })
  return '-'
}

function rowEta(row) {
  if (row.status === 'done') return '-'
  const size = Number(row.size) || 0
  if (!size) return '-'
  const remain = Math.max(0, size - (row.loaded || 0))
  if (!remain) return '0s'
  const speed = row.speed || 0
  if (speed <= 0) return '-'
  return formatDuration((remain / speed) * 1000)
}

function rowElapsed(row) {
  if (!row.startedAt) return '-'
  return formatDuration((row.finishedAt || props.nowTick || Date.now()) - row.startedAt)
}

function close() {
  if (props.uploading) {
    ElMessage.warning(t('filetransfer.stillUploading'))
    return
  }
  visible.value = false
}
</script>

<style scoped>
.upload-summary { display: flex; flex-wrap: wrap; gap: 14px; margin-bottom: 12px; color: #606266; font-size: 13px; }
.upload-summary + .el-table { margin-top: 0; }
:deep(.el-dialog__footer) .el-button { display: inline-flex; align-items: center; gap: 4px; }
</style>
