import { rewriteWs } from '../../session/sessionWs'
import { t } from '../../i18n'

/**
 * One-shot exec WebSocket helper (Agent update / opsctl-style run).
 * Callers own ticket issuance; this only drives the WS protocol.
 *
 * @param {string} browserWs ticket browserWs (will be rewriteWs'd)
 * @param {string} command
 * @param {{
 *   onOutput?: (chunk: string) => void,
 *   onStatus?: (text: string) => void,
 *   hasLog?: () => boolean,
 *   timeoutSec?: number
 * }} [handlers]
 * @returns {{ close: () => void, done: Promise<void> }}
 */
export function openExecWs(browserWs, command, handlers = {}) {
  const {
    onOutput = () => {},
    onStatus = () => {},
    hasLog = () => false,
    timeoutSec = 600
  } = handlers

  let ws = null
  let settled = false

  const close = () => {
    if (ws) {
      try { ws.close() } catch { /* ignore */ }
      ws = null
    }
  }

  const done = new Promise((resolve) => {
    const finish = () => {
      if (settled) return
      settled = true
      resolve()
    }

    ws = new WebSocket(rewriteWs(browserWs))
    ws.onopen = () => {
      ws.send(JSON.stringify({
        type: 'run',
        command,
        timeoutSec
      }))
    }
    ws.onmessage = (ev) => {
      let msg
      try {
        msg = JSON.parse(typeof ev.data === 'string' ? ev.data : '')
      } catch {
        onOutput(String(ev.data))
        return
      }
      if (msg.type === 'output' && msg.data) {
        onOutput(msg.data)
        return
      }
      if (msg.type === 'done') {
        const code = msg.exitCode
        onOutput(`\n[done] exitCode=${code ?? '?'} durationMs=${msg.durationMs ?? '?'}\n`)
        onStatus(
          code === 0
            ? t('exec.installDoneVerifying')
            : t('exec.endedStillVerifying', { code })
        )
        finish()
        return
      }
      if (msg.type === 'error') {
        onOutput(`\n[error] ${msg.error || 'exec error'}\n`)
        onStatus(t('exec.failed'))
        finish()
      }
    }
    ws.onerror = () => {
      if (!settled) {
        onOutput(t('exec.wsErrorOutput'))
        onStatus(t('exec.connectFailed'))
      }
      finish()
    }
    ws.onclose = () => {
      if (!settled) {
        if (hasLog()) {
          onOutput(t('exec.closedRestartOk'))
          onStatus(t('exec.disconnectedRestart'))
        } else {
          onOutput(t('exec.closed'))
          onStatus(t('exec.disconnected'))
        }
      }
      finish()
    }
  })

  return { close, done }
}
