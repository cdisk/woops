/**
 * Binary filetransfer client (Browser ↔ Gateway /ws/file-transfer ↔ Agent).
 * Control frames are JSON text; payloads are OPST binary frames (no base64).
 */

import api from '../../shared/api'
import { rewriteWs } from '../../session/sessionWs'
import { formatBytes } from '../../shared/format'
import { t } from '../../i18n'

export const XFER_CHUNK = 1024 * 1024
export const XFER_MAX_AUTO_RETRIES = 6
export const BLOB_DOWNLOAD_LIMIT = 64 * 1024 * 1024
const TASK_STORE_KEY = 'ops.filetransfer.tasks'

const MAGIC = [0x4f, 0x50, 0x53, 0x54] // OPST
const HEADER_LEN = 50

function uuid() {
  if (crypto.randomUUID) return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

async function sha256Hex(bytes) {
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('')
}

export async function fileFingerprint(file) {
  const size = file.size || 0
  const mtime = file.lastModified || 0
  const headN = Math.min(65536, size)
  const head = new Uint8Array(await file.slice(0, headN).arrayBuffer())
  let tail = head
  if (size > 65536) {
    tail = new Uint8Array(await file.slice(size - 65536, size).arrayBuffer())
  }
  const buf = new Uint8Array(head.length + tail.length)
  buf.set(head, 0)
  buf.set(tail, head.length)
  const hash = await sha256Hex(buf)
  return `v1;size=${size};mtime=${mtime};hash=${hash}`
}

export async function bytesFingerprint(bytes) {
  const size = bytes.length
  const headN = Math.min(65536, size)
  const head = bytes.subarray(0, headN)
  const tail = size > 65536 ? bytes.subarray(size - 65536) : head
  const buf = new Uint8Array(head.length + tail.length)
  buf.set(head, 0)
  buf.set(tail, head.length)
  const hash = await sha256Hex(buf)
  return `v1;size=${size};mtime=0;hash=${hash}`
}

function encodeChunk(offset, data) {
  if (!data.length || data.length > XFER_CHUNK) {
    throw new Error(`invalid chunk length ${data.length}`)
  }
  return sha256Hex(data).then((hex) => {
    const out = new Uint8Array(HEADER_LEN + data.length)
    out[0] = MAGIC[0]
    out[1] = MAGIC[1]
    out[2] = MAGIC[2]
    out[3] = MAGIC[3]
    out[4] = 1
    out[5] = 0
    const view = new DataView(out.buffer)
    view.setBigUint64(6, BigInt(offset), false)
    view.setUint32(14, data.length, false)
    for (let i = 0; i < 32; i++) {
      out[18 + i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
    }
    out.set(data, HEADER_LEN)
    return out
  })
}

function decodeChunk(frame) {
  if (!(frame instanceof Uint8Array)) frame = new Uint8Array(frame)
  if (frame.length < HEADER_LEN) throw new Error('short binary frame')
  if (frame[0] !== MAGIC[0] || frame[1] !== MAGIC[1] || frame[2] !== MAGIC[2] || frame[3] !== MAGIC[3]) {
    throw new Error('bad magic')
  }
  if (frame[4] !== 1) throw new Error('bad version')
  const view = new DataView(frame.buffer, frame.byteOffset, frame.byteLength)
  const offset = Number(view.getBigUint64(6, false))
  const length = view.getUint32(14, false)
  if (length <= 0 || length > XFER_CHUNK || HEADER_LEN + length !== frame.length) {
    throw new Error('bad length')
  }
  const data = frame.subarray(HEADER_LEN)
  return sha256Hex(data).then((hex) => {
    for (let i = 0; i < 32; i++) {
      const b = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
      if (frame[18 + i] !== b) throw new Error('chunk checksum mismatch')
    }
    return { offset, data }
  })
}

function waitMessage(ws, timeoutMs = 120000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup()
      reject(new Error(t('filetransfer.transferTimeout')))
    }, timeoutMs)
    const onMsg = (ev) => {
      cleanup()
      resolve(ev)
    }
    const onErr = () => {
      cleanup()
      reject(new Error(t('filetransfer.wsError')))
    }
    const onClose = () => {
      cleanup()
      reject(new Error(t('filetransfer.transferDisconnected')))
    }
    const cleanup = () => {
      clearTimeout(timer)
      ws.removeEventListener('message', onMsg)
      ws.removeEventListener('error', onErr)
      ws.removeEventListener('close', onClose)
    }
    ws.addEventListener('message', onMsg)
    ws.addEventListener('error', onErr)
    ws.addEventListener('close', onClose)
  })
}

async function issueTicket(assetId, fields) {
  const { data } = await api.post('/sessions/ticket', {
    assetId,
    protocol: 'filetransfer',
    ...fields
  })
  if (!data?.browserWs) throw new Error(data?.message || t('filetransfer.createSessionFailed'))
  return data
}

function openWs(browserWs) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(rewriteWs(browserWs))
    ws.binaryType = 'arraybuffer'
    const timer = setTimeout(() => {
      ws.close()
      reject(new Error(t('filetransfer.connectTimeout')))
    }, 20000)
    ws.onopen = () => {
      clearTimeout(timer)
      resolve(ws)
    }
    ws.onerror = () => {
      clearTimeout(timer)
      reject(new Error(t('filetransfer.connectFailed')))
    }
  })
}

async function readControl(ws) {
  const ev = await waitMessage(ws)
  if (typeof ev.data !== 'string') {
    // Unexpected binary while waiting for control — surface as disconnect-ish.
    throw new Error('unexpected binary frame')
  }
  return JSON.parse(ev.data)
}

/**
 * Upload a Browser File with auto-reconnect / resume.
 * onProgress({ loaded, total, status, error })
 */
export async function uploadFile({
  assetId,
  remotePath,
  file,
  transferId,
  fingerprint,
  signal,
  onProgress
}) {
  const size = file.size || 0
  const fp = fingerprint || (await fileFingerprint(file))
  let tid = transferId || uuid()
  let lastErr = null
  let lastOffset = 0

  for (let attempt = 0; attempt < XFER_MAX_AUTO_RETRIES; attempt++) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    if (attempt > 0) {
      await sleep(Math.min(8000, 400 * 2 ** (attempt - 1)))
    }
    let ws
    try {
      const ticket = await issueTicket(assetId, {
        direction: 'upload',
        path: remotePath,
        size,
        fingerprint: fp,
        transferId: tid
      })
      tid = ticket.transferId || tid
      ws = await openWs(ticket.browserWs)
      const status = await readControl(ws)
      if (status.type === 'error') throw new Error(status.message || t('filetransfer.transferFailed'))
      if (status.type !== 'status') throw new Error(`unexpected ${status.type}`)
      let offset = Number(status.offset) || 0
      if (offset > size) throw new Error(t('filetransfer.remoteOffsetTooLarge'))
      lastOffset = offset
      onProgress?.({
        loaded: offset,
        total: size,
        status: 'uploading',
        phase: 'start',
        transferId: tid,
        resumed: !!status.resumed
      })

      while (offset < size) {
        if (signal?.aborted) {
          await sendCtrl(ws, { type: 'abort' })
          throw new DOMException('Aborted', 'AbortError')
        }
        const end = Math.min(offset + XFER_CHUNK, size)
        const slice = new Uint8Array(await file.slice(offset, end).arrayBuffer())
        const frame = await encodeChunk(offset, slice)
        ws.send(frame)
        const ack = await readControl(ws)
        if (ack.type === 'error') throw new Error(ack.message || t('filetransfer.chunkWriteFailed'))
        if (ack.type !== 'ack') throw new Error(`unexpected ${ack.type}`)
        offset = Number(ack.offset) || offset + slice.length
        lastOffset = offset
        onProgress?.({ loaded: offset, total: size, status: 'uploading', transferId: tid })
      }

      // Agent verifies size; client sha256 optional (omit to avoid holding large files in memory).
      await sendCtrl(ws, { type: 'commit' })
      const done = await readControl(ws)
      if (done.type === 'error') throw new Error(done.message || t('filetransfer.commitFailed'))
      if (done.type !== 'committed' || !done.ok) throw new Error(t('filetransfer.commitFailed'))
      onProgress?.({ loaded: size, total: size, status: 'done', transferId: tid })
      ws.close()
      return { transferId: tid, fingerprint: fp, sha256: done.sha256 || '' }
    } catch (e) {
      lastErr = e
      try { ws?.close() } catch { /* ignore */ }
      if (e?.name === 'AbortError') throw e
      onProgress?.({ loaded: lastOffset, total: size, status: 'retrying', transferId: tid, error: e.message })
    }
  }
  onProgress?.({ loaded: lastOffset, total: size, status: 'paused', transferId: tid, error: lastErr?.message })
  throw lastErr || new Error(t('filetransfer.uploadFailed'))
}

/**
 * Upload raw bytes (text editor save).
 */
export async function uploadBytes({ assetId, remotePath, bytes, signal, onProgress }) {
  const blob = new Blob([bytes])
  const file = new File([blob], 'content.bin', { type: 'application/octet-stream' })
  return uploadFile({ assetId, remotePath, file, signal, onProgress })
}

/**
 * Download remote file. Prefer File System Access API; else Blob under limit.
 */
export async function downloadFile({
  assetId,
  remotePath,
  fileName,
  transferId,
  signal,
  onProgress
}) {
  let tid = transferId || uuid()
  let localOffset = 0
  let writable = null
  let sinkChunks = null
  let useFsa = false
  let lastErr = null
  let fingerprint = ''
  let total = 0

  if (window.showSaveFilePicker) {
    try {
      onProgress?.({ loaded: 0, total: 0, status: 'downloading', phase: 'pick-save', transferId: tid })
      const handle = await window.showSaveFilePicker({
        suggestedName: fileName || 'download.bin'
      })
      writable = await handle.createWritable()
      useFsa = true
    } catch (e) {
      if (e?.name === 'AbortError') throw e
      // fall through to blob mode
    }
  }
  if (!useFsa) {
    sinkChunks = []
  }

  for (let attempt = 0; attempt < XFER_MAX_AUTO_RETRIES; attempt++) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    if (attempt > 0) await sleep(Math.min(8000, 400 * 2 ** (attempt - 1)))
    let ws
    try {
      const ticket = await issueTicket(assetId, {
        direction: 'download',
        path: remotePath,
        transferId: tid
      })
      tid = ticket.transferId || tid
      ws = await openWs(ticket.browserWs)
      const status = await readControl(ws)
      if (status.type === 'error') throw new Error(status.message || t('filetransfer.downloadFailed'))
      if (status.type !== 'status') throw new Error(`unexpected ${status.type}`)
      total = Number(status.size) || 0
      fingerprint = status.fingerprint || fingerprint
      if (!useFsa && total > BLOB_DOWNLOAD_LIMIT) {
        throw new Error(t('filetransfer.streamSaveUnsupported', { size: formatBytes(total) }))
      }
      if (localOffset > total) localOffset = 0
      onProgress?.({ loaded: localOffset, total, status: 'downloading', phase: 'start', transferId: tid })

      await sendCtrl(ws, { type: 'request', offset: localOffset, fingerprint })
      for (;;) {
        if (signal?.aborted) {
          await sendCtrl(ws, { type: 'abort' })
          throw new DOMException('Aborted', 'AbortError')
        }
        const ev = await waitMessage(ws)
        if (typeof ev.data === 'string') {
          const ctrl = JSON.parse(ev.data)
          if (ctrl.type === 'committed') {
            onProgress?.({ loaded: total, total, status: 'done', transferId: tid })
            ws.close()
            if (useFsa) {
              await writable.close()
            } else {
              const blob = new Blob(sinkChunks)
              triggerBlobDownload(blob, fileName || 'download.bin')
            }
            return { transferId: tid, size: total }
          }
          if (ctrl.type === 'error') throw new Error(ctrl.message || t('filetransfer.downloadFailed'))
          if (ctrl.type === 'aborted') throw new Error(t('filetransfer.cancelled'))
          continue
        }
        const frame = new Uint8Array(ev.data)
        const { offset, data } = await decodeChunk(frame)
        if (offset !== localOffset) throw new Error(`unexpected offset ${offset}`)
        if (useFsa) {
          await writable.write(data)
        } else {
          sinkChunks.push(data)
        }
        localOffset += data.length
        await sendCtrl(ws, { type: 'ack', offset: localOffset, ok: true })
        onProgress?.({ loaded: localOffset, total, status: 'downloading', transferId: tid })
      }
    } catch (e) {
      lastErr = e
      try { ws?.close() } catch { /* ignore */ }
      if (e?.name === 'AbortError') {
        try { await writable?.abort?.() } catch { /* ignore */ }
        throw e
      }
      onProgress?.({ loaded: localOffset, total, status: 'retrying', transferId: tid, error: e.message })
    }
  }
  try { await writable?.abort?.() } catch { /* ignore */ }
  throw lastErr || new Error(t('filetransfer.downloadFailed'))
}

/**
 * Download into memory (text editor). Rejects large files.
 */
export async function downloadBytes({ assetId, remotePath, maxBytes, signal }) {
  const chunks = []
  let loaded = 0
  let tid = uuid()
  const ticket = await issueTicket(assetId, { direction: 'download', path: remotePath, transferId: tid })
  tid = ticket.transferId || tid
  const ws = await openWs(ticket.browserWs)
  try {
    const status = await readControl(ws)
    if (status.type === 'error') throw new Error(status.message || t('filetransfer.readFailed'))
    const total = Number(status.size) || 0
    if (maxBytes && total > maxBytes) {
      throw new Error(t('filetransfer.fileTooLarge', { size: formatBytes(maxBytes) }))
    }
    await sendCtrl(ws, { type: 'request', offset: 0, fingerprint: status.fingerprint })
    for (;;) {
      if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
      const ev = await waitMessage(ws)
      if (typeof ev.data === 'string') {
        const ctrl = JSON.parse(ev.data)
        if (ctrl.type === 'committed') break
        if (ctrl.type === 'error') throw new Error(ctrl.message || t('filetransfer.readFailed'))
        continue
      }
      const { offset, data } = await decodeChunk(new Uint8Array(ev.data))
      if (offset !== loaded) throw new Error('unexpected offset')
      chunks.push(data)
      loaded += data.length
      if (maxBytes && loaded > maxBytes) throw new Error(t('filetransfer.fileTooLarge', { size: formatBytes(maxBytes) }))
      await sendCtrl(ws, { type: 'ack', offset: loaded, ok: true })
    }
  } finally {
    try { ws.close() } catch { /* ignore */ }
  }
  const out = new Uint8Array(loaded)
  let pos = 0
  for (const c of chunks) {
    out.set(c, pos)
    pos += c.length
  }
  return out
}

export async function abortTransfer({ assetId, remotePath, transferId }) {
  if (!transferId) return
  const ticket = await issueTicket(assetId, {
    direction: 'upload',
    path: remotePath,
    transferId,
    abort: true,
    size: 0,
    fingerprint: ''
  })
  const ws = await openWs(ticket.browserWs)
  try {
    await readControl(ws)
  } catch {
    // ignore
  } finally {
    try { ws.close() } catch { /* ignore */ }
  }
}

function sendCtrl(ws, obj) {
  ws.send(JSON.stringify(obj))
  return Promise.resolve()
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}

function triggerBlobDownload(blob, name) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** Persist light-weight upload task metadata (no file bytes). */
export function loadStoredTasks(assetId) {
  try {
    const all = JSON.parse(localStorage.getItem(TASK_STORE_KEY) || '{}')
    return Array.isArray(all[assetId]) ? all[assetId] : []
  } catch {
    return []
  }
}

export function saveStoredTasks(assetId, tasks) {
  try {
    const all = JSON.parse(localStorage.getItem(TASK_STORE_KEY) || '{}')
    all[assetId] = tasks.map((t) => ({
      id: t.id,
      transferId: t.transferId,
      relativePath: t.relativePath,
      remotePath: t.remotePath,
      size: t.size,
      fingerprint: t.fingerprint,
      status: t.status,
      loaded: t.loaded,
      error: t.error || ''
    }))
    localStorage.setItem(TASK_STORE_KEY, JSON.stringify(all))
  } catch {
    // ignore quota
  }
}
