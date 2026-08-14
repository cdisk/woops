/**
 * JSON-RPC over a filemanager WebSocket (Browser ↔ Gateway ↔ Agent).
 * One pending map + incremental id; reject all on disconnect.
 */
import { t } from '../../i18n'

export function createFileRpc() {
  let ws = null
  let nextId = 1
  const pending = new Map()

  function attach(socket) {
    ws = socket
  }

  function rejectAll(reason) {
    const err = reason instanceof Error ? reason : new Error(String(reason || t('files.disconnected')))
    for (const p of pending.values()) p.reject(err)
    pending.clear()
  }

  function handleMessage(raw) {
    let msg
    try {
      msg = JSON.parse(raw)
    } catch {
      return false
    }
    if (msg?.id == null || !pending.has(msg.id)) return false
    const p = pending.get(msg.id)
    pending.delete(msg.id)
    if (msg.error) {
      const err =
        typeof msg.error === 'string'
          ? msg.error
          : msg.error.message || JSON.stringify(msg.error)
      p.reject(new Error(err))
    } else {
      p.resolve(msg.result)
    }
    return true
  }

  function rpc(method, params, timeoutMs = 60000) {
    return new Promise((resolve, reject) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) {
        reject(new Error(t('files.wsNotConnected')))
        return
      }
      const id = nextId++
      const timer = setTimeout(() => {
        if (!pending.has(id)) return
        pending.delete(id)
        reject(new Error(t('files.rpcTimeout', { method, sec: Math.round(timeoutMs / 1000) })))
      }, timeoutMs)
      pending.set(id, {
        resolve: (v) => {
          clearTimeout(timer)
          resolve(v)
        },
        reject: (e) => {
          clearTimeout(timer)
          reject(e)
        }
      })
      try {
        ws.send(JSON.stringify({ id, method, params }))
      } catch (e) {
        pending.delete(id)
        clearTimeout(timer)
        reject(e instanceof Error ? e : new Error(String(e)))
      }
    })
  }

  return { attach, rejectAll, handleMessage, rpc }
}
