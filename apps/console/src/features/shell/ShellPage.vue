<template>
  <div class="term-page">
    <div class="bar">
      <el-button @click="goBack">{{ t('shell.close') }}</el-button>
      <SessionAssetTitle :title="title" :kind="kindLabel" :asset="asset" />
      <el-tag size="small" :type="statusTagType">{{ statusLabel }}</el-tag>
      <el-button size="small" :title="t('shell.ctrlWTitle')" @click="sendCtrl('w')">Ctrl+W</el-button>
      <el-button size="small" :title="t('shell.ctrlTTitle')" @click="sendCtrl('t')">Ctrl+T</el-button>
      <ShellCommands :asset-id="String(route.params.assetId || '')" @run="runCommand" />
    </div>
    <div class="term-wrap">
      <div ref="termRef" class="term"></div>
      <SessionConnectingMask :visible="status === 'connecting'" :text="t('shell.connecting')" />
    </div>

    <el-dialog
      v-model="closedDialogVisible"
      :title="t('shell.disconnectedTitle')"
      width="420px"
      align-center
      :close-on-click-modal="true"
      :close-on-press-escape="true"
    >
      <p class="closed-msg">{{ t('shell.disconnectedMsg') }}</p>
      <template #footer>
        <el-button @click="closedDialogVisible = false">{{ t('shell.gotIt') }}</el-button>
        <el-button type="primary" @click="goBack">{{ t('shell.closeTab') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { ElMessage } from 'element-plus'
import SessionAssetTitle from '../../session/SessionAssetTitle.vue'
import SessionConnectingMask from '../../session/SessionConnectingMask.vue'
import { closeSessionTab, rewriteWs } from '../../session/sessionWs'
import { requestShellTicket } from './ticket'
import ShellCommands from './ShellCommands.vue'

const { t } = useI18n()
const route = useRoute()
const termRef = ref(null)
const title = ref('Shell')
const asset = ref(null)
const status = ref('connecting')
const closedDialogVisible = ref(false)
let term, fit, ws, resizeObserver
let intentionalClose = false

const KIND_LABEL = { bash: 'Shell', powershell: 'PowerShell', cmd: 'CMD' }

const kindLabel = computed(() => {
  const k = resolveKind()
  return KIND_LABEL[k] || k
})
const statusLabel = computed(() => {
  const map = {
    connecting: t('shell.statusConnecting'),
    connected: t('shell.statusConnected'),
    closed: t('shell.statusClosed'),
    error: t('shell.statusError')
  }
  return map[status.value] || status.value
})
const statusTagType = computed(() => {
  if (status.value === 'connected') return 'success'
  if (status.value === 'error' || status.value === 'closed') return 'danger'
  return 'warning'
})

function resolveKind() {
  const k = String(route.query.kind || 'bash').toLowerCase()
  if (k === 'powershell' || k === 'bash' || k === 'cmd') return k
  return 'bash'
}

const goBack = closeSessionTab

function markClosed() {
  if (intentionalClose) return
  if (status.value === 'closed') return
  const wasConnected = status.value === 'connected'
  status.value = 'closed'
  if (wasConnected) closedDialogVisible.value = true
}

onMounted(async () => {
  const kind = resolveKind()
  title.value = kindLabel.value

  term = new Terminal({
    cursorBlink: true,
    fontFamily: 'Consolas, Menlo, monospace',
    fontSize: 14,
    theme: { background: '#0b1020' },
    // ConPTY xterm backend only for modern Windows PowerShell; WinPTY/cmd must not use it.
    ...(kind === 'powershell' ? { windowsPty: { backend: 'conpty' } } : {})
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(termRef.value)
  fit.fit()
  term.focus()

  try {
    const data = await requestShellTicket(route.params.assetId, kind)
    if (data.implemented === false) {
      const fallback = t('shell.notReady', { kind: kindLabel.value })
      ElMessage.warning(data.message || fallback)
      status.value = 'error'
      term.writeln(`\r\n${data.message || fallback}\r\n`)
      return
    }
    if (!data.browserWs) {
      ElMessage.error(t('shell.ticketNoBrowserWs'))
      status.value = 'error'
      return
    }
    const name = data.asset?.displayName
    asset.value = data.asset || null
    title.value = name ? `${name} · ${kindLabel.value}` : kindLabel.value
    // Same-origin through Console (nginx /ws → gateway INTERNAL); see rewriteWs.
    ws = new WebSocket(rewriteWs(data.browserWs))
    ws.binaryType = 'arraybuffer'
    ws.onopen = () => {
      status.value = 'connected'
      // Agent waits for the first R,cols,rows before spawning ConPTY; send ASAP
      // and re-assert after layout settles (late Resize alone is unreliable on ConPTY).
      fit.fit()
      sendResize()
      setTimeout(() => { fit.fit(); sendResize() }, 50)
      setTimeout(() => { fit.fit(); sendResize() }, 250)
      setTimeout(() => { fit.fit(); sendResize() }, 1000)
    }
    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        if (ev.data.startsWith('ERROR')) {
          ElMessage.error(ev.data)
          status.value = 'error'
        }
        term.write(ev.data)
        return
      }
      term.write(new Uint8Array(ev.data))
    }
    ws.onclose = () => { markClosed() }
    ws.onerror = () => {
      if (status.value === 'connected') {
        markClosed()
        return
      }
      status.value = 'error'
    }

    term.onData((data) => {
      if (ws?.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(data))
    })
    termRef.value?.addEventListener('click', () => term.focus())
    resizeObserver = new ResizeObserver(() => {
      fit.fit()
      sendResize()
    })
    resizeObserver.observe(termRef.value)
  } catch (e) {
    const msg = e.response?.data?.error || e.response?.data?.message || e.message || t('shell.createFailed')
    ElMessage.error(msg)
    status.value = 'error'
    term?.writeln(`\r\n${msg}\r\n`)
  }
})

function sendResize() {
  if (!term || !ws || ws.readyState !== WebSocket.OPEN) return
  ws.send(`R,${term.cols},${term.rows}`)
}

function sendCtrl(letter) {
  const ch = String(letter || '').toLowerCase()
  if (ch.length !== 1 || ch < 'a' || ch > 'z') return
  if (ws?.readyState === WebSocket.OPEN) {
    ws.send(new Uint8Array([ch.charCodeAt(0) & 0x1f]))
  }
  term?.focus()
}

function runCommand(cmd) {
  if (ws?.readyState !== WebSocket.OPEN) {
    ElMessage.warning(t('shell.notConnected'))
    return
  }
  let text = String(cmd ?? '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  if (!text) return
  if (!text.endsWith('\n')) text += '\n'
  ws.send(new TextEncoder().encode(text))
  term?.focus()
}

onBeforeUnmount(() => {
  intentionalClose = true
  closedDialogVisible.value = false
  resizeObserver?.disconnect()
  ws?.close()
  term?.dispose()
})
</script>

<style scoped>
.term-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 10px 12px;
  box-sizing: border-box;
  background: var(--ops-bg-page);
}
.bar {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 10px;
  padding: 8px 12px;
  background: var(--ops-bg-surface);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  flex-shrink: 0;
}
.bar :deep(.session-titles) { flex: 1; min-width: 120px; }
.term-wrap { position: relative; flex: 1; min-height: 0; }
.term {
  height: 100%;
  background: #0b1020;
  padding: 8px;
  border-radius: var(--ops-radius);
  box-sizing: border-box;
  border: 1px solid var(--ops-border);
}
.closed-msg { margin: 0; line-height: 1.6; color: var(--el-text-color-regular); }

/* xterm scrollback — thin dark track matching the terminal chrome */
.term :deep(.xterm-viewport) {
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.45) transparent;
}
.term :deep(.xterm-viewport::-webkit-scrollbar) {
  width: 8px;
}
.term :deep(.xterm-viewport::-webkit-scrollbar-track) {
  background: transparent;
  margin: 4px 0;
}
.term :deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: rgba(148, 163, 184, 0.35);
  border-radius: 999px;
  border: 2px solid transparent;
  background-clip: padding-box;
}
.term :deep(.xterm-viewport::-webkit-scrollbar-thumb:hover) {
  background: rgba(148, 163, 184, 0.55);
  background-clip: padding-box;
}
.term :deep(.xterm-viewport::-webkit-scrollbar-corner) {
  background: transparent;
}
</style>
