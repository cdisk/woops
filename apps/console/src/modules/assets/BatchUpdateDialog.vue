<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('assets.batchUpdateTitle')"
    width="720px"
    align-center
    :close-on-click-modal="!running"
    :close-on-press-escape="!running"
    :before-close="beforeClose"
    @close="emit('update:modelValue', false)"
  >
    <p class="hint">{{ t('assets.batchUpdateHint') }}</p>
    <p v-if="running" class="live">{{ t('assets.batchStayOpen') }}</p>
    <p v-if="errorText" class="error">{{ errorText }}</p>
    <div v-loading="loading" class="layers">
      <section v-for="(layer, index) in layers" :key="index" class="layer">
        <h4>{{ t('assets.batchLayer', { n: index + 1 }) }}</h4>
        <ul>
          <li v-for="asset in layer.assets || []" :key="asset.id">
            <div class="who">
              <span class="name">{{ asset.displayName || asset.id }}</span>
              <span class="ip">{{ asset.privateIp || t('common.emDash') }}</span>
            </div>
            <div class="state">
              <el-tag size="small" :type="tagType(statusOf(asset.id))">{{ statusLabel(statusOf(asset.id)) }}</el-tag>
              <span v-if="messageOf(asset.id)" class="msg">{{ messageOf(asset.id) }}</span>
            </div>
          </li>
        </ul>
      </section>
    </div>
    <template #footer>
      <el-button :disabled="running" @click="requestClose">{{ t('common.close') }}</el-button>
      <el-button type="primary" :disabled="loading || !!errorText || !layers.length" :loading="running" @click="start">
        {{ t('assets.startUpdate') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { runAgentUpdate } from './agentUpdate'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  assets: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue', 'done'])

const { t } = useI18n()
const CONCURRENCY = 3
const loading = ref(false)
const running = ref(false)
const layers = ref([])
const rowState = ref({})
const errorText = ref('')
const closers = new Set()
let aborted = false

watch(() => props.modelValue, (open) => {
  if (open) loadPlan()
})

function statusOf(id) {
  return rowState.value[id]?.status || 'pending'
}

function messageOf(id) {
  return rowState.value[id]?.message || ''
}

function statusLabel(status) {
  const key = {
    pending: 'assets.batchPending',
    downloading: 'assets.batchDownloading',
    verifying: 'assets.batchVerifying',
    ok: 'assets.batchOk',
    failed: 'assets.batchFailed',
    blocked: 'assets.batchBlocked',
    skipped: 'assets.batchSkipped'
  }[status] || 'assets.batchPending'
  return t(key)
}

function tagType(status) {
  if (status === 'ok') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'blocked' || status === 'skipped') return 'info'
  if (status === 'downloading' || status === 'verifying') return 'warning'
  return 'info'
}

function phaseStatus(phase) {
  if (phase === 'verifying') return 'verifying'
  if (phase === 'success') return 'ok'
  if (phase === 'warn' || phase === 'error') return 'failed'
  if (phase === 'downloading' || phase === 'info') return 'downloading'
  return ''
}

async function loadPlan() {
  aborted = false
  closers.clear()
  loading.value = true
  errorText.value = ''
  layers.value = []
  rowState.value = {}
  try {
    const { data } = await api.post('/assets/update-plan', {
      assetIds: props.assets.map((asset) => asset.id)
    })
    layers.value = data?.layers || []
    const next = {}
    for (const layer of layers.value) {
      for (const asset of layer.assets || []) next[asset.id] = { status: 'pending', message: '' }
    }
    rowState.value = next
  } catch (error) {
    errorText.value = error?.response?.data?.error || error?.message || t('assets.planFailed')
  } finally {
    loading.value = false
  }
}

async function runPool(items, worker) {
  const queue = [...items]
  const workers = Array.from({ length: Math.min(CONCURRENCY, queue.length) }, async () => {
    while (queue.length && !aborted) {
      const item = queue.shift()
      if (item) await worker(item)
    }
  })
  await Promise.all(workers)
}

function onLeave(event) {
  if (!running.value) return
  event.preventDefault()
  event.returnValue = ''
}

async function start() {
  if (running.value || !layers.value.length) return
  aborted = false
  running.value = true
  window.addEventListener('beforeunload', onLeave)
  const skip = new Set()
  try {
    for (const layer of layers.value) {
      if (aborted) break
      await runPool(layer.assets || [], async (asset) => {
        if (aborted) return
        if (skip.has(asset.id)) {
          rowState.value[asset.id] = { status: 'blocked', message: '' }
          return
        }
        const result = await runAgentUpdate(asset, {
          shouldAbort: () => aborted,
          bindClose: (fn) => { if (typeof fn === 'function') closers.add(fn) },
          onPhase: (phase) => {
            const status = phaseStatus(phase)
            if (status) rowState.value[asset.id] = { ...rowState.value[asset.id], status }
          },
          onStatus: (text) => {
            rowState.value[asset.id] = { ...rowState.value[asset.id], message: text }
          }
        })
        let status = 'failed'
        let message = result.message || rowState.value[asset.id]?.message || ''
        if (result.status === 'ok') status = 'ok'
        else if (result.status === 'skipped') {
          status = 'skipped'
          message = result.reason === 'ungrouped'
            ? t('assets.batchSkippedUngrouped')
            : t('assets.batchSkippedOffline')
        } else if (result.status === 'aborted') status = 'failed'
        rowState.value[asset.id] = { status, message }
        const blocking = result.started && result.status !== 'ok' && result.status !== 'skipped'
        if (blocking) {
          for (const id of asset.blockIds || []) skip.add(id)
        }
      })
    }
  } finally {
    running.value = false
    window.removeEventListener('beforeunload', onLeave)
    emit('done')
  }
}

function abortRuns() {
  aborted = true
  for (const close of closers) {
    try { close() } catch { /* already closed */ }
  }
  closers.clear()
}

async function beforeClose(done) {
  if (!running.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('assets.batchCloseWarn'), t('assets.batchUpdateTitle'), {
      type: 'warning',
      confirmButtonText: t('common.close'),
      cancelButtonText: t('common.cancel')
    })
    abortRuns()
    done()
  } catch {
    /* keep the dialog open */
  }
}

function requestClose() {
  beforeClose(() => emit('update:modelValue', false))
}

onBeforeUnmount(() => {
  abortRuns()
  window.removeEventListener('beforeunload', onLeave)
})
</script>

<style scoped>
.hint, .live { margin: 0 0 8px; color: var(--ops-text-secondary); font-size: 13px; line-height: 1.5; }
.live { color: var(--ops-warning); }
.error { margin: 0 0 8px; color: var(--ops-danger); font-size: 13px; }
.layers { max-height: 420px; overflow: auto; }
.layer + .layer { margin-top: 12px; }
.layer h4 { margin: 0 0 6px; font-size: 13px; }
.layer ul { list-style: none; margin: 0; padding: 0; }
.layer li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 0;
  border-bottom: 1px solid var(--ops-border);
}
.state { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; min-width: 0; }
.msg { font-size: 11px; color: var(--ops-text-muted); text-align: right; }
.who { min-width: 0; }
.name { display: block; font-size: 13px; }
.ip { display: block; font-size: 11px; color: var(--ops-text-muted); }
</style>
