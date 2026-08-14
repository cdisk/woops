<template>
  <div ref="pageRef" class="page" :class="{ 'is-fs': isFullscreen }">
    <div class="head">
      <div class="titles">
        <el-button link @click="back">{{ t('audit.play.back') }}</el-button>
        <h2>{{ t('audit.play.title') }}</h2>
        <span v-if="meta" class="meta">{{ meta }}</span>
      </div>
      <div class="head-actions">
        <template v-if="isGuac && durationMs > 0">
          <el-button size="small" :disabled="!ready" @click="togglePlay">
            {{ playing ? t('audit.play.pause') : t('audit.play.play') }}
          </el-button>
          <span class="pos">{{ formatMs(positionMs) }} / {{ formatMs(durationMs) }}</span>
          <el-slider
            v-model="seekMs"
            :min="0"
            :max="Math.max(durationMs, 1)"
            :disabled="!ready"
            style="width: 220px"
            @change="onSeek"
          />
        </template>
        <el-button
          v-if="hasRecording && !error && !notice"
          size="small"
          @click="toggleFullscreen"
        >
          {{ isFullscreen ? t('audit.play.exitFullscreen') : t('audit.play.fullscreen') }}
        </el-button>
        <el-button v-if="hasRecording" size="small" :loading="downloading" @click="download">
          {{ t('audit.play.download') }}
        </el-button>
      </div>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <el-alert
      v-else-if="notice"
      :title="notice"
      type="info"
      show-icon
      :closable="false"
    />
    <div
      v-show="!error && !notice"
      ref="playerRef"
      v-loading="loading"
      class="player"
      :class="{ guac: isGuac }"
    />
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import Guacamole from 'guacamole-common-js'
import api from '../../../shared/api'
import { formatDateTime, operationTypeLabel, recordingPath } from '../shared/operationFormat'
import 'asciinema-player/dist/bundle/asciinema-player.css'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const pageRef = ref(null)
const playerRef = ref(null)
const loading = ref(true)
const downloading = ref(false)
const error = ref('')
const notice = ref('')
const operation = ref(null)
const ready = ref(false)
const playing = ref(false)
const positionMs = ref(0)
const durationMs = ref(0)
const seekMs = ref(0)
const isGuac = ref(false)
const isFullscreen = ref(false)

let castPlayer = null
let guacRecording = null
let guacDisplay = null
let blobUrl = ''
let posTimer = null
let resizeObserver = null
let fitFrame = 0

const hasRecording = computed(() => !!recordingPath(operation.value))
const meta = computed(() => {
  const op = operation.value
  if (!op) return ''
  const asset = op.assetDisplayName || op.hostname || op.assetId
  return [operationTypeLabel(op.operationType), asset, op.username, formatDateTime(op.occurredAt)]
    .filter(Boolean)
    .join(' · ')
})

function back() {
  router.push(`/audit/operations/${route.params.id}`)
}

function formatMs(ms) {
  const s = Math.floor(Number(ms || 0) / 1000)
  const m = Math.floor(s / 60)
  const r = s % 60
  return `${m}:${String(r).padStart(2, '0')}`
}

async function fetchRecording() {
  const { data } = await api.get(`/server-operations/${route.params.id}/recording`, {
    responseType: 'blob'
  })
  return data
}

async function download() {
  downloading.value = true
  try {
    const blob = await fetchRecording()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = recordingPath(operation.value).split('/').pop() || 'recording'
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('audit.play.downloadFailed'))
  } finally {
    downloading.value = false
  }
}

function fitGuac() {
  if (!guacDisplay || !playerRef.value) return
  const w = guacDisplay.getWidth()
  const h = guacDisplay.getHeight()
  if (w <= 0 || h <= 0) return
  // Leave one physical-pixel margin so fractional scaling never rounds up into
  // a scrollbar. Guacamole scales its internal display through a compositor
  // transform and updates the outer bounds to this same scaled size.
  const cw = playerRef.value.clientWidth - 2
  const ch = playerRef.value.clientHeight - 2
  if (cw <= 0 || ch <= 0) return
  const scale = Math.min(cw / w, ch / h)
  guacDisplay.scale(Math.max(0.05, scale))
}

function scheduleFitGuac() {
  if (!isGuac.value || fitFrame) return
  fitFrame = requestAnimationFrame(() => {
    fitFrame = 0
    fitGuac()
  })
}

function watchPlayerSize() {
  if (!playerRef.value || typeof ResizeObserver === 'undefined') return
  resizeObserver?.disconnect()
  resizeObserver = new ResizeObserver(scheduleFitGuac)
  resizeObserver.observe(playerRef.value)
}

function onFullscreenChange() {
  const el = pageRef.value
  isFullscreen.value = !!(el && document.fullscreenElement === el)
  scheduleFitGuac()
}

async function toggleFullscreen() {
  const el = pageRef.value
  if (!el) return
  try {
    if (document.fullscreenElement === el) {
      await document.exitFullscreen?.()
    } else {
      await el.requestFullscreen?.()
    }
  } catch (e) {
    ElMessage.error(e?.message || t('audit.play.fullscreenFailed'))
  }
}

async function playCast() {
  const blob = await fetchRecording()
  blobUrl = URL.createObjectURL(blob)
  const AsciinemaPlayer = await import('asciinema-player')
  // Let flex layout establish the final viewport before asciinema measures it.
  await nextTick()
  await new Promise((resolve) => requestAnimationFrame(resolve))
  // fit:both needs a fixed-height container; overflow:hidden avoids scrollbar flicker
  // that fit:width + overflow:auto can cause (scrollbar ↔ shrink ↔ hide loop).
  castPlayer = AsciinemaPlayer.create(blobUrl, playerRef.value, {
    fit: 'both',
    idleTimeLimit: 3
  })
  watchPlayerSize()
}

async function playGuac() {
  isGuac.value = true
  const raw = await fetchRecording()
  // Axios may give a Blob that is fine; normalize so SessionRecording always gets one.
  const blob =
    raw instanceof Blob ? raw : new Blob([raw], { type: 'application/octet-stream' })
  if (!blob.size) {
    throw new Error(t('audit.play.emptyRecording'))
  }
  guacRecording = new Guacamole.SessionRecording(blob)
  const display = guacRecording.getDisplay()
  guacDisplay = display
  const el = display.getElement()
  // Do not set height:auto / maxWidth on the Guacamole root — it owns pixel
  // width/height via resize+scale. Overriding height collapses the desktop
  // canvas to empty while the software cursor layer still moves (black screen).
  el.style.flex = '0 0 auto'
  el.style.maxWidth = '100%'
  el.style.maxHeight = '100%'
  el.style.overflow = 'hidden'
  el.style.contain = 'layout paint'
  if (el.firstElementChild) {
    el.firstElementChild.style.willChange = 'transform'
    el.firstElementChild.style.backfaceVisibility = 'hidden'
  }
  playerRef.value.innerHTML = ''
  playerRef.value.appendChild(el)

  display.onresize = scheduleFitGuac

  guacRecording.onplay = () => {
    playing.value = true
  }
  guacRecording.onpause = () => {
    playing.value = false
  }
  guacRecording.onseek = (ms) => {
    positionMs.value = ms
    seekMs.value = ms
  }

  // Prefer full parse (onload). onprogress(duration, parsedChars) — do not treat
  // the 2nd arg as milliseconds (that was a prior bug).
  await new Promise((resolve, reject) => {
    let settled = false
    const done = () => {
      if (settled) return
      settled = true
      resolve()
    }
    const failTimer = setTimeout(() => {
      if (!settled) {
        settled = true
        reject(new Error(t('audit.play.parseTimeout')))
      }
    }, 60000)
    guacRecording.onload = () => {
      clearTimeout(failTimer)
      try {
        const d = guacRecording.getDuration?.()
        if (typeof d === 'number' && d > 0) durationMs.value = d
      } catch {
        /* ignore */
      }
      done()
    }
    guacRecording.onprogress = (duration) => {
      if (typeof duration === 'number' && duration > durationMs.value) {
        durationMs.value = duration
      }
    }
    guacRecording.onerror = (msg) => {
      clearTimeout(failTimer)
      if (!settled) {
        settled = true
        reject(new Error(msg || t('audit.play.parseFailed')))
      }
    }
  })

  ready.value = true
  watchPlayerSize()
  await nextTick()
  scheduleFitGuac()
  guacRecording.play()
  posTimer = setInterval(() => {
    try {
      positionMs.value = guacRecording.getPosition?.() ?? positionMs.value
      seekMs.value = positionMs.value
      const d = guacRecording.getDuration?.()
      if (typeof d === 'number' && d > durationMs.value) durationMs.value = d
    } catch {
      /* ignore */
    }
  }, 250)
}

function togglePlay() {
  if (!guacRecording) return
  if (guacRecording.isPlaying?.()) {
    guacRecording.pause()
  } else {
    guacRecording.play()
  }
}

function onSeek(ms) {
  if (!guacRecording) return
  guacRecording.seek(ms, () => {
    positionMs.value = ms
  })
}

onMounted(async () => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  window.addEventListener('resize', scheduleFitGuac, { passive: true })
  try {
    const { data } = await api.get(`/server-operations/${route.params.id}`)
    operation.value = data?.operation || null
    const format = operation.value?.detail?.format
    if (!recordingPath(operation.value)) {
      notice.value = t('audit.play.noRecording')
      return
    }
    if (format === 'guacamole') {
      await playGuac()
      return
    }
    if (format !== 'asciinema') {
      notice.value = t('audit.play.unsupportedFormat', {
        format: format || t('audit.play.unknownFormat')
      })
      return
    }
    await playCast()
  } catch (e) {
    error.value = e?.response?.data?.error || e?.message || t('audit.play.loadFailed')
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  window.removeEventListener('resize', scheduleFitGuac)
  resizeObserver?.disconnect()
  resizeObserver = null
  if (fitFrame) cancelAnimationFrame(fitFrame)
  fitFrame = 0
  if (document.fullscreenElement && pageRef.value && document.fullscreenElement === pageRef.value) {
    document.exitFullscreen?.().catch(() => {})
  }
  if (posTimer) clearInterval(posTimer)
  castPlayer?.dispose?.()
  try {
    guacRecording?.pause?.()
  } catch {
    /* ignore */
  }
  guacRecording = null
  guacDisplay = null
  if (blobUrl) URL.revokeObjectURL(blobUrl)
})
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  padding: 4px 0;
  box-sizing: border-box;
  overflow: hidden;
}
.page.is-fs {
  width: 100vw;
  height: 100vh;
  padding: 12px;
  background: #111;
}
.head {
  display: flex; align-items: center; justify-content: space-between;
  gap: 12px; margin-bottom: 12px; flex-wrap: wrap; flex-shrink: 0;
}
.titles { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.titles h2 { margin: 0; font-size: 18px; }
.page.is-fs .titles h2 { color: #e5e7eb; }
.page.is-fs .meta { color: #9ca3af; }
.meta { color: #6b7280; font-size: 12px; }
.head-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.pos { font-size: 12px; color: #4b5563; font-variant-numeric: tabular-nums; min-width: 72px; }
.page.is-fs .pos { color: #d1d5db; }
.player {
  flex: 1;
  width: 100%;
  min-width: 0;
  min-height: 0;
  background: #111;
  border-radius: 4px;
  overflow: hidden;
  contain: layout paint size;
}
.player.guac {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  background: #0b0b0b;
  overflow: hidden;
}
.player.guac > :deep(*) {
  flex: 0 0 auto;
  max-width: 100%;
  max-height: 100%;
}
.page.is-fs .player { border-radius: 0; }
</style>
