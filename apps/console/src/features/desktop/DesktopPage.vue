<template>
  <div class="rdp-page" :class="{ 'is-page-fs': isPageFs }">
    <div v-show="!isPageFs" class="bar">
      <div class="bar-left">
        <el-button @click="goBack">{{ t('desktop.close') }}</el-button>
        <SessionAssetTitle :title="title" :kind="protocolLabel" :asset="asset" />
        <el-tag type="warning" size="small">{{ protocolLabel }}</el-tag>
        <el-tag v-if="status" size="small" :type="statusTagType">{{ statusLabel }}</el-tag>
      </div>
      <div v-if="status === 'ready'" class="bar-right">
        <el-button-group>
          <el-button size="small" :title="t('desktop.sendCtrlAltDel')" @click="sendSpecialKeys([KEY.CTRL, KEY.ALT, KEY.DELETE])">
            Ctrl+Alt+Del
          </el-button>
          <el-button size="small" :title="t('desktop.sendWin')" @click="sendSpecialKeys([KEY.WIN])">Win</el-button>
          <el-button size="small" :title="t('desktop.sendAltTab')" @click="sendSpecialKeys([KEY.ALT, KEY.TAB])">Alt+Tab</el-button>
          <el-button size="small" :title="t('desktop.sendCtrlEsc')" @click="sendSpecialKeys([KEY.CTRL, KEY.ESC])">
            Ctrl+Esc
          </el-button>
          <el-button size="small" :title="t('desktop.sendEsc')" @click="sendSpecialKeys([KEY.ESC])">Esc</el-button>
        </el-button-group>
        <el-popover placement="bottom-end" :width="480" trigger="click">
          <template #reference>
            <el-button size="small">{{ t('desktop.clipboard') }}</el-button>
          </template>
          <div class="clipboard-panel">
            <el-input
              v-model="clipboardText"
              type="textarea"
              :rows="7"
              resize="vertical"
              :placeholder="t('desktop.clipboardPlaceholder')"
            />
            <div class="clipboard-actions">
              <el-button size="small" type="primary" @click="sendClipboardEditor">{{ t('desktop.sendToRemote') }}</el-button>
              <el-button size="small" @click="syncBrowserClipboardToRemote">{{ t('desktop.readLocalClipboard') }}</el-button>
              <el-button size="small" @click="copyRemoteClipboardToLocal">{{ t('desktop.copyRemoteToLocal') }}</el-button>
            </div>
            <p class="clipboard-status">{{ clipboardStatus }}</p>
            <p class="clipboard-limit">
              {{ t('desktop.clipboardLimit') }}
            </p>
          </div>
        </el-popover>
        <el-button size="small" type="primary" plain @click="enterPageFs">{{ t('desktop.fullscreen') }}</el-button>
      </div>
    </div>

    <div v-if="status !== 'ready'" class="progress-box">
      <el-steps :active="stepIndex" finish-status="success" :process-status="error ? 'error' : 'process'" align-center>
        <el-step :title="t('desktop.stepTicket')" />
        <el-step :title="t('desktop.stepTunnel')" />
        <el-step :title="t('desktop.stepDesktop')" />
        <el-step :title="t('desktop.stepReady')" />
      </el-steps>
      <p class="progress-hint">{{ statusDetail }}</p>
      <p v-if="!error && status !== 'closed'" class="progress-sub">{{ waitingHint }}</p>
    </div>

    <el-result v-if="error" icon="warning" :title="t('desktop.unavailable', { protocol: protocolLabel })" :sub-title="error">
      <template #extra>
        <el-button type="primary" @click="goBack">{{ t('desktop.closeTab') }}</el-button>
      </template>
    </el-result>

    <div v-show="!error" ref="viewport" class="viewport" :class="{ dimmed: status !== 'ready' }" />

    <DesktopFsRail
      v-if="isPageFs && status === 'ready'"
      v-model:clipboard-text="clipboardText"
      :clipboard-status="clipboardStatus"
      @exit="exitPageFs"
      @special="sendSpecialKeys"
      @clipboard-send="sendClipboardEditor"
      @clipboard-read-local="syncBrowserClipboardToRemote()"
      @clipboard-copy-local="copyRemoteClipboardToLocal"
    />
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import Guacamole from 'guacamole-common-js'
import api from '../../shared/api'
import SessionAssetTitle from '../../session/SessionAssetTitle.vue'
import DesktopFsRail from './DesktopFsRail.vue'
import { closeSessionTab } from '../../session/sessionWs'
import { splitDesktopWs } from './desktopWs'

const { t } = useI18n()
const route = useRoute()
const title = ref('Desktop')
const asset = ref(null)
const status = ref('init')
const statusDetail = ref('')
const error = ref('')
const viewport = ref(null)
const clipboardText = ref('')
const clipboardStatus = ref('')
const isPageFs = ref(false)

let client
let tunnel
let keyboard
let mouse
let resizeObserver
let resizeTimer
let connected = false
let inputElement
let remoteClipboard

const MAX_CLIPBOARD_BYTES = 256 * 1024
const KEY = {
  CTRL: 0xFFE3,
  ALT: 0xFFE9,
  WIN: 0xFFEB,
  ESC: 0xFF1B,
  TAB: 0xFF09,
  DELETE: 0xFFFF,
  V: 0x76
}

const protocol = computed(() => (route.meta.protocol === 'vnc' ? 'vnc' : 'rdp'))
const protocolLabel = computed(() => protocol.value.toUpperCase())

const statusLabel = computed(() => {
  const map = {
    init: t('desktop.statusInit'),
    ticket: t('desktop.statusTicket'),
    connecting: t('desktop.statusConnecting'),
    waiting: t('desktop.statusWaiting'),
    ready: t('desktop.statusReady'),
    error: t('desktop.statusError'),
    closed: t('desktop.statusClosed')
  }
  return map[status.value] || status.value
})
const statusTagType = computed(() => {
  if (status.value === 'ready') return 'success'
  if (status.value === 'error') return 'danger'
  if (status.value === 'closed') return 'info'
  return 'warning'
})

const stepIndex = computed(() => {
  if (error.value) {
    switch (status.value) {
      case 'ticket': return 0
      case 'connecting': return 1
      case 'waiting': return 2
      default: return 2
    }
  }
  switch (status.value) {
    case 'init': return 0
    case 'ticket': return 0
    case 'connecting': return 1
    case 'waiting': return 2
    case 'ready': return 4
    default: return 0
  }
})

const goBack = closeSessionTab

const waitingHint = computed(() =>
  protocol.value === 'vnc' ? t('desktop.waitVnc') : t('desktop.waitRdp')
)

function humanizeError(raw) {
  const msg = String(raw || '')
  if (/invalid credentials|Authentication failure/i.test(msg)) {
    return protocol.value === 'vnc' ? t('desktop.authFailVnc') : t('desktop.authFailRdp')
  }
  if (/wrong security type/i.test(msg)) {
    return t('desktop.securityFail')
  }
  if (/password|密码/i.test(msg)) {
    return msg
  }
  return msg
}

function fail(msg) {
  const text = humanizeError(msg)
  error.value = text
  status.value = 'error'
  statusDetail.value = text
  releaseInput()
  ElMessage.error(text)
}

function releaseInput() {
  if (inputElement) {
    inputElement.removeEventListener('keydown', handleClipboardShortcut, true)
    document.removeEventListener('paste', handlePasteEvent, true)
  }
  if (keyboard) {
    try { keyboard.onkeydown = null } catch { /* ignore */ }
    try { keyboard.onkeyup = null } catch { /* ignore */ }
  }
  keyboard = null
  mouse = null
  inputElement = null
}

function attachInput(el) {
  releaseInput()
  inputElement = el
  el.tabIndex = 0
  el.addEventListener('keydown', handleClipboardShortcut, true)
  document.addEventListener('paste', handlePasteEvent, true)
  mouse = new Guacamole.Mouse(el)
  const display = client.getDisplay()
  display.oncursor = (canvas, x, y) => {
    if (mouse?.setCursor(canvas, x, y)) display.showCursor(false)
    else display.showCursor(true)
  }
  mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (state) => {
    if (state.left) el.focus({ preventScroll: true })
    client.sendMouseState(state, true)
  }
  // Viewport only — do not capture document-level keys (browser Ctrl+C etc.).
  keyboard = new Guacamole.Keyboard(el)
  keyboard.onkeydown = (keysym) => client.sendKeyEvent(1, keysym)
  keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym)
}

function textFromClipboardData(data) {
  if (!data) return ''
  const plain = data.getData('text/plain')
  if (plain) return plain
  const html = data.getData('text/html')
  if (!html) return ''
  const doc = new DOMParser().parseFromString(html, 'text/html')
  return doc.body?.innerText || doc.body?.textContent || ''
}

function sendRemotePasteShortcut() {
  sendSpecialKeys([KEY.CTRL, KEY.V])
}

/** Press keys in order, then release in reverse — for combos browsers cannot send. */
function sendSpecialKeys(keysyms) {
  if (!client || !connected || !Array.isArray(keysyms) || keysyms.length === 0) return
  for (const keysym of keysyms) client.sendKeyEvent(1, keysym)
  for (let i = keysyms.length - 1; i >= 0; i--) client.sendKeyEvent(0, keysyms[i])
  inputElement?.focus({ preventScroll: true })
}

function sendTextToRemote(text, pasteAfter = false) {
  if (!client || !connected) {
    return Promise.reject(new Error(t('desktop.notConnected')))
  }

  const blob = new Blob([String(text ?? '')], { type: 'text/plain;charset=utf-8' })
  if (blob.size > MAX_CLIPBOARD_BYTES) {
    return Promise.reject(new Error(t('desktop.clipboardTooLarge')))
  }

  return new Promise((resolve, reject) => {
    const stream = client.createClipboardStream('text/plain')
    const writer = new Guacamole.StringWriter(stream)
    writer.onack = (ack) => {
      if (ack?.isError?.()) {
        clipboardStatus.value = ack.message || t('desktop.clipboardRejected')
        ElMessage.warning(clipboardStatus.value)
      }
    }
    try {
      writer.sendText(String(text ?? ''))
      writer.sendEnd()
      clipboardText.value = String(text ?? '')
      clipboardStatus.value = t('desktop.clipboardSent', { n: blob.size })
      // guacd's RDP clipboard handler does not ACK this client→server stream.
      // Allow CLIPRDR to advertise the new format before injecting Ctrl+V.
      if (pasteAfter) window.setTimeout(sendRemotePasteShortcut, 100)
      resolve()
    } catch (err) {
      reject(err)
    }
  })
}

async function handlePasteText(text, pasteAfter) {
  try {
    await sendTextToRemote(text, pasteAfter)
  } catch (err) {
    ElMessage.warning(err?.message || t('desktop.clipboardSendFailed'))
  }
}

function handleClipboardShortcut(event) {
  if (!connected || !(event.ctrlKey || event.metaKey) || event.altKey ||
    String(event.key).toLowerCase() !== 'v') {
    return
  }
  event.stopImmediatePropagation()
  if (window.isSecureContext && navigator.clipboard?.readText) {
    event.preventDefault()
    syncBrowserClipboardToRemote(true)
  } else {
    clipboardStatus.value = t('desktop.clipboardWaitPaste')
  }
}

function handlePasteEvent(event) {
  if (!connected) return
  const target = event.target
  if (target !== inputElement &&
    (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target?.isContentEditable)) {
    return
  }
  const text = textFromClipboardData(event.clipboardData)
  if (!text && !event.clipboardData?.types?.includes('text/plain')) {
    ElMessage.warning(t('desktop.clipboardNoText'))
    return
  }
  event.preventDefault()
  event.stopImmediatePropagation()
  handlePasteText(text, true)
}

async function syncBrowserClipboardToRemote(pasteAfter = false) {
  if (!navigator.clipboard?.readText) {
    ElMessage.warning(t('desktop.clipboardReadBlocked'))
    return
  }
  try {
    const text = await navigator.clipboard.readText()
    await handlePasteText(text, pasteAfter)
  } catch {
    ElMessage.warning(t('desktop.clipboardReadDenied'))
  }
}

function sendClipboardEditor() {
  handlePasteText(clipboardText.value, false)
}

async function writeRemoteClipboardToBrowser(showError = false) {
  if (!remoteClipboard) {
    if (showError) ElMessage.info(t('desktop.clipboardNoRemote'))
    return false
  }
  try {
    const { blob, mimetype, text } = remoteClipboard
    if (mimetype === 'text/plain' || mimetype.startsWith('text/plain;')) {
      const value = text ?? await blob.text()
      if (window.isSecureContext && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value)
      } else if (!legacyCopyText(value)) {
        throw new Error(t('desktop.clipboardWriteDenied'))
      }
    } else if (navigator.clipboard?.write && window.ClipboardItem &&
      ['text/html', 'image/png'].includes(mimetype)) {
      await navigator.clipboard.write([new window.ClipboardItem({ [mimetype]: blob })])
    } else {
      throw new Error(t('desktop.clipboardWriteUnsupported', { mimetype }))
    }
    clipboardStatus.value = t('desktop.clipboardSynced', { mimetype })
    return true
  } catch (err) {
    clipboardStatus.value = t('desktop.clipboardCached')
    if (showError) ElMessage.warning(err?.message || t('desktop.clipboardWriteFailed'))
    return false
  }
}

function legacyCopyText(text) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  let copied = false
  try { copied = document.execCommand('copy') } catch { /* ignore */ }
  textarea.remove()
  inputElement?.focus({ preventScroll: true })
  return copied
}

function copyRemoteClipboardToLocal() {
  writeRemoteClipboardToBrowser(true)
}

function receiveRemoteClipboard(stream, mimetype) {
  const reader = new Guacamole.BlobReader(stream, mimetype)
  reader.onend = async () => {
    const blob = reader.getBlob()
    const normalizedType = String(mimetype || blob.type || 'application/octet-stream').toLowerCase()
    let text
    if (normalizedType.startsWith('text/')) {
      text = await blob.text()
      clipboardText.value = text
    }
    remoteClipboard = { blob, mimetype: normalizedType, text }
    clipboardStatus.value = t('desktop.clipboardReceived', { type: normalizedType, n: blob.size })
    await writeRemoteClipboardToBrowser(false)
  }
}

function fitDisplay() {
  if (!client || !viewport.value) return
  const display = client.getDisplay()
  const width = display.getWidth()
  const height = display.getHeight()
  if (width <= 0 || height <= 0) return
  const scale = Math.min(
    viewport.value.clientWidth / width,
    viewport.value.clientHeight / height
  )
  display.scale(Math.max(0.1, scale))
}

/**
 * Ready: use flex leftover viewport (100% − header). Connecting: estimate from
 * page − bar so progress box does not shrink the negotiated size.
 */
function measureDesktopSize() {
  const el = viewport.value
  if (status.value === 'ready' && el && el.clientWidth > 0 && el.clientHeight > 0) {
    return {
      width: Math.max(800, Math.min(8192, Math.floor(el.clientWidth))),
      height: Math.max(600, Math.min(8192, Math.floor(el.clientHeight)))
    }
  }
  const page = el?.closest?.('.rdp-page') || el?.parentElement
  if (page) {
    const cs = getComputedStyle(page)
    const padX = (parseFloat(cs.paddingLeft) || 0) + (parseFloat(cs.paddingRight) || 0)
    const padY = (parseFloat(cs.paddingTop) || 0) + (parseFloat(cs.paddingBottom) || 0)
    const bar = isPageFs.value ? null : page.querySelector?.('.bar')
    const barGap = bar ? 8 : 0
    const barH = (bar?.offsetHeight || 0) + barGap
    return {
      width: Math.max(800, Math.min(8192, Math.floor(page.clientWidth - padX))),
      height: Math.max(600, Math.min(8192, Math.floor(page.clientHeight - padY - barH)))
    }
  }
  return { width: 1280, height: 800 }
}

function requestRemoteSize() {
  if (!connected || !client) return
  const { width, height } = measureDesktopSize()
  try { client.sendSize(width, height) } catch { /* ignore */ }
}

/** After layout settles (progress box gone), nudge RDP display-update. */
function syncRemoteSizeAfterReady() {
  nextTick(() => {
    requestRemoteSize()
    fitDisplay()
    window.clearTimeout(resizeTimer)
    resizeTimer = window.setTimeout(() => {
      requestRemoteSize()
      fitDisplay()
    }, 300)
  })
}

function enterPageFs() {
  if (status.value !== 'ready') return
  isPageFs.value = true
  syncRemoteSizeAfterReady()
}

function exitPageFs() {
  isPageFs.value = false
  syncRemoteSizeAfterReady()
}

function handleUnhandledRejection(event) {
  const message = String(event?.reason?.message || event?.reason || '')
  if (/image|bitmap|decode/i.test(message)) {
    fail(t('desktop.decodeFailed', { message }))
  }
}

onMounted(async () => {
  statusDetail.value = t('desktop.preparing')
  clipboardStatus.value = t('desktop.clipboardWait')
  try {
    window.addEventListener('unhandledrejection', handleUnhandledRejection)

    status.value = 'ticket'
    statusDetail.value = t('desktop.applyingTicket')
    const { data } = await api.post('/sessions/ticket', {
      assetId: route.params.assetId,
      protocol: protocol.value
    })
    asset.value = data.asset || null
    title.value = data.asset?.displayName
      ? `${data.asset.displayName} · ${protocolLabel.value}`
      : protocolLabel.value

    if (!data.browserWs) {
      fail(data.message || t('desktop.channelNotReady', { protocol: protocolLabel.value }))
      return
    }

    // Gateway opens the standard Guacamole WebSocket tunnel and configures guacd.
    status.value = 'waiting'
    statusDetail.value = protocol.value === 'vnc'
      ? t('desktop.buildingVnc')
      : t('desktop.buildingRdp')
    const { base, connectData } = splitDesktopWs(data.browserWs)
    const { width, height } = measureDesktopSize()
    const sizeQuery = `width=${width}&height=${height}`
    const connectWithSize = connectData ? `${connectData}&${sizeQuery}` : sizeQuery
    tunnel = new Guacamole.WebSocketTunnel(base)
    tunnel.receiveTimeout = 120000
    tunnel.unstableThreshold = 60000
    client = new Guacamole.Client(tunnel)
    client.onclipboard = receiveRemoteClipboard

    // Mount display early so the first framebuffer paints into a visible element.
    const displayEl = client.getDisplay().getElement()
    displayEl.classList.add('guacamole-display')
    viewport.value?.appendChild(displayEl)
    client.getDisplay().onresize = fitDisplay
    resizeObserver = new ResizeObserver(() => {
      fitDisplay()
      window.clearTimeout(resizeTimer)
      resizeTimer = window.setTimeout(requestRemoteSize, 200)
    })
    if (viewport.value) resizeObserver.observe(viewport.value)

    tunnel.onerror = (statusObj) => {
      const detail = statusObj?.message || (statusObj?.code != null ? `code ${statusObj.code}` : '')
      fail(detail
        ? t('desktop.tunnelErrorDetail', { protocol: protocolLabel.value, detail })
        : t('desktop.tunnelError', { protocol: protocolLabel.value }))
    }
    client.onerror = (statusObj) => {
      const detail = statusObj?.message || (statusObj?.code != null ? `code ${statusObj.code}` : '')
      fail(detail || t('desktop.sessionError', { protocol: protocolLabel.value }))
    }
    client.onstatechange = (state) => {
      // 0 idle, 1 connecting, 2 waiting, 3 connected, 4 disconnecting, 5 disconnected
      if (error.value) return
      if (state === 1) {
        status.value = 'connecting'
        statusDetail.value = t('desktop.tunnelHandshake')
      } else if (state === 2) {
        status.value = 'waiting'
        statusDetail.value = t('desktop.connectingTarget', { protocol: protocolLabel.value })
      } else if (state === 3) {
        connected = true
        status.value = 'ready'
        statusDetail.value = t('desktop.desktopConnected')
        const el = client.getDisplay().getElement()
        if (viewport.value && el && !viewport.value.contains(el)) {
          viewport.value.appendChild(el)
        }
        syncRemoteSizeAfterReady()
        attachInput(el)
      } else if (state === 5 && status.value !== 'error') {
        connected = false
        status.value = 'closed'
        statusDetail.value = t('desktop.connectionClosed')
        releaseInput()
      }
    }

    client.connect(connectWithSize)
  } catch (e) {
    const msg = e.response?.data?.error || e.response?.data?.message || e.message ||
      t('desktop.createFailed', { protocol: protocolLabel.value })
    fail(msg)
  }
})

onBeforeUnmount(() => {
  connected = false
  isPageFs.value = false
  releaseInput()
  resizeObserver?.disconnect()
  window.clearTimeout(resizeTimer)
  window.removeEventListener('unhandledrejection', handleUnhandledRejection)
  try { client?.disconnect() } catch { /* ignore */ }
  tunnel = null
  client = null
})
</script>

<style scoped>
.rdp-page {
  position: relative;
  height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 8px 12px;
  box-sizing: border-box;
  background: #111;
  color: #e5e7eb;
  overflow: hidden;
}
.rdp-page.is-page-fs {
  padding: 0;
}
.bar {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  flex-wrap: wrap;
  flex-shrink: 0;
}
.bar-left, .bar-right {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.bar-left { flex: 1; min-width: 0; }
.bar-left :deep(.session-titles) { flex: 1; min-width: 120px; }
.bar-left :deep(.sub-line) { color: #94a3b8; }
.bar-left :deep(.title-line) { color: #e5e7eb; }
.progress-box {
  margin: 48px auto 12px;
  max-width: 720px;
  width: 100%;
  padding: 28px 20px 20px;
  background: #1a1a1a;
  border: 1px solid #333;
  border-radius: 10px;
  flex-shrink: 0;
}
.progress-hint {
  text-align: center;
  color: #fbbf24;
  margin: 20px 0 0;
  font-size: 15px;
  font-weight: 500;
}
.progress-sub {
  text-align: center;
  color: #9ca3af;
  margin: 8px 0 0;
  font-size: 13px;
}
.progress-box :deep(.el-step__title) { color: #d1d5db; font-size: 13px; }
.progress-box :deep(.el-step__title.is-process) { color: #fbbf24; }
.progress-box :deep(.el-step__title.is-success) { color: #86efac; }
.progress-box :deep(.el-step__title.is-error) { color: #f87171; }
.viewport {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: #000;
  display: flex;
  align-items: center;
  justify-content: center;
}
.viewport.dimmed { opacity: 0.35; }
.viewport :deep(.guacamole-display) {
  position: relative;
  transform-origin: 0 0;
  flex: 0 0 auto;
}
.clipboard-panel { display: flex; flex-direction: column; gap: 10px; }
.clipboard-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.clipboard-status { margin: 0; color: #409eff; font-size: 12px; }
.clipboard-limit { margin: 0; color: #909399; font-size: 12px; line-height: 1.5; }
</style>
