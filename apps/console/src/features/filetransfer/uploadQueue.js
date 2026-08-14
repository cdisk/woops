/**
 * Upload queue for the file manager page: task list, sequential runner,
 * resume/cancel and throughput metering.
 *
 * Path semantics (Windows drives, roots, mkdir) stay with the caller via
 * `prepareRemotePath`, so this module only deals with the queue itself.
 */

import { ref } from 'vue'
import {
  abortTransfer,
  fileFingerprint,
  loadStoredTasks,
  saveStoredTasks,
  uploadFile
} from './transferClient'
import { createRateMeter, resetRateMeter, sampleRate } from './rateMeter'
import { t } from '../../i18n'

const TICK_MS = 500

export function useUploadQueue({ assetId, prepareRemotePath, onQueueSettled }) {
  const items = ref([])
  const uploading = ref(false)
  const nowTick = ref(Date.now())
  const overallSpeed = ref(0)

  const abortControllers = new Map()
  const meters = new Map()
  const overallMeter = createRateMeter()
  // Only counts bytes actually pushed in this page session; resumed offsets are baselines, not throughput.
  let sessionBytes = 0
  let seq = 0
  let tickTimer = null

  function meterFor(id) {
    let m = meters.get(id)
    if (!m) {
      m = createRateMeter()
      meters.set(id, m)
    }
    return m
  }

  // Re-sampling on every tick lets a stalled transfer decay to 0 instead of
  // freezing on the last observed rate.
  function sampleMeters(now) {
    for (const item of items.value) {
      if (item.status === 'uploading' || item.status === 'retrying') {
        item.speed = sampleRate(meterFor(item.id), item.loaded || 0, now)
      }
    }
    overallSpeed.value = sampleRate(overallMeter, sessionBytes, now)
  }

  function startTick() {
    stopTick()
    tickTimer = setInterval(() => {
      nowTick.value = Date.now()
      sampleMeters(nowTick.value)
    }, TICK_MS)
  }

  function stopTick() {
    if (tickTimer) {
      clearInterval(tickTimer)
      tickTimer = null
    }
  }

  function persist() {
    if (!assetId.value) return
    saveStoredTasks(assetId.value, items.value)
  }

  function restoreStored() {
    const stored = loadStoredTasks(assetId.value)
    if (!stored.length) return
    items.value = stored.map((task) => ({
      ...task,
      id: ++seq,
      file: null,
      speed: 0,
      runBytes: 0,
      startedAt: 0,
      finishedAt: 0,
      status: task.status === 'done' ? 'done' : 'paused',
      error: task.file ? task.error : (task.error || t('filetransfer.reselectFile'))
    }))
  }

  async function queueFiles(fileList) {
    const queued = []
    for (const file of fileList) {
      const relativePath = file.webkitRelativePath || file.name
      const fp = await fileFingerprint(file)
      const existing = items.value.find(
        (i) =>
          (i.status === 'paused' || i.status === 'error') &&
          i.relativePath === relativePath &&
          (!i.fingerprint || i.fingerprint === fp)
      )
      if (existing) {
        existing.file = file
        existing.fingerprint = fp
        existing.size = file.size || 0
        existing.status = 'pending'
        existing.error = ''
        queued.push(existing)
        continue
      }
      queued.push({
        id: ++seq,
        file,
        relativePath,
        remotePath: '',
        size: file.size || 0,
        loaded: 0,
        speed: 0,
        runBytes: 0,
        status: 'pending',
        startedAt: 0,
        finishedAt: 0,
        error: '',
        transferId: '',
        fingerprint: fp
      })
    }
    const keep = items.value.filter((i) => !queued.includes(i))
    items.value = [...keep, ...queued.filter((i) => !keep.includes(i))]
    for (const it of queued) {
      if (!items.value.includes(it)) items.value.push(it)
    }
    startQueue()
    return queued.length
  }

  async function startQueue() {
    if (uploading.value) return
    uploading.value = true
    resetRateMeter(overallMeter, sessionBytes)
    overallSpeed.value = 0
    startTick()
    try {
      for (const item of items.value) {
        if (item.status !== 'pending') continue
        await uploadOne(item)
        persist()
      }
      await onQueueSettled?.()
    } finally {
      uploading.value = false
      stopTick()
      nowTick.value = Date.now()
      overallSpeed.value = 0
      persist()
    }
  }

  async function resumeFailed() {
    for (const item of items.value) {
      if (item.status === 'error' || item.status === 'paused') {
        if (!item.file) {
          item.error = t('filetransfer.reselectAfterRefresh')
          continue
        }
        item.status = 'pending'
        item.error = ''
      }
    }
    await startQueue()
  }

  async function cancelAll() {
    for (const [id, ac] of abortControllers.entries()) {
      ac.abort()
      abortControllers.delete(id)
    }
    for (const item of items.value) {
      if (item.status === 'uploading' || item.status === 'retrying' || item.status === 'pending' || item.status === 'paused') {
        if (item.transferId && item.remotePath) {
          try {
            await abortTransfer({
              assetId: assetId.value,
              remotePath: item.remotePath,
              transferId: item.transferId
            })
          } catch {
            // best effort
          }
        }
        item.status = 'error'
        item.error = t('filetransfer.cancelled')
        item.finishedAt = Date.now()
      }
    }
    persist()
  }

  function onItemProgress(item, p) {
    if (p.transferId) item.transferId = p.transferId
    const now = Date.now()
    const loaded = Number(p.loaded) || 0
    if (p.phase === 'start') {
      // Remote offset after (re)connect: a baseline, not bytes we just sent.
      item.loaded = loaded
      item.speed = resetRateMeter(meterFor(item.id), loaded, now)
    } else if (loaded > (item.loaded || 0)) {
      const delta = loaded - item.loaded
      item.loaded = loaded
      item.runBytes = (item.runBytes || 0) + delta
      sessionBytes += delta
      item.speed = sampleRate(meterFor(item.id), loaded, now)
      overallSpeed.value = sampleRate(overallMeter, sessionBytes, now)
    }
    if (p.status === 'retrying') {
      item.status = 'retrying'
      item.speed = resetRateMeter(meterFor(item.id), item.loaded || 0, now)
    } else if (p.status === 'uploading') item.status = 'uploading'
    else if (p.status === 'paused') item.status = 'paused'
    if (p.error) item.error = p.error
  }

  async function uploadOne(item) {
    item.status = 'uploading'
    item.startedAt = Date.now()
    item.error = ''
    item.loaded = item.loaded || 0
    item.speed = 0
    item.runBytes = 0
    resetRateMeter(meterFor(item.id), item.loaded, item.startedAt)
    const ac = new AbortController()
    abortControllers.set(item.id, ac)
    try {
      const remote = await prepareRemotePath(item.relativePath)
      item.remotePath = remote
      if (!item.fingerprint && item.file) {
        item.fingerprint = await fileFingerprint(item.file)
      }
      await uploadFile({
        assetId: assetId.value,
        remotePath: remote,
        file: item.file,
        transferId: item.transferId || undefined,
        fingerprint: item.fingerprint,
        signal: ac.signal,
        onProgress: (p) => onItemProgress(item, p)
      })
      item.status = 'done'
      item.finishedAt = Date.now()
      item.loaded = item.size
      item.error = ''
    } catch (e) {
      if (e?.name === 'AbortError') {
        item.status = 'error'
        item.error = t('filetransfer.cancelled')
      } else {
        item.status = item.transferId ? 'paused' : 'error'
        item.error = e.message || t('filetransfer.uploadFailed')
      }
      item.finishedAt = Date.now()
    } finally {
      abortControllers.delete(item.id)
      meters.delete(item.id)
      item.speed = 0
    }
  }

  function dispose() {
    stopTick()
  }

  return {
    items,
    uploading,
    nowTick,
    overallSpeed,
    restoreStored,
    queueFiles,
    resumeFailed,
    cancelAll,
    dispose
  }
}
