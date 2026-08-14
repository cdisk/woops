import { formatBytes, formatDateTime } from '../../../shared/format'
import { t } from '../../../i18n'

export const OPERATION_TYPES = [
  { value: 'SHELL', get label() { return 'Shell' } },
  { value: 'FILE', get label() { return t('audit.format.typeFile') } },
  { value: 'EXEC', get label() { return t('audit.format.typeExec') } },
  { value: 'RDP', get label() { return t('audit.format.typeRdp') } },
  { value: 'VNC', get label() { return 'VNC' } },
  { value: 'PORTMAP_TCP', get label() { return t('audit.format.typePortmapTcp') } },
  { value: 'PORTMAP_UDP', get label() { return t('audit.format.typePortmapUdp') } }
]

export const SESSION_OPERATION_TYPES = OPERATION_TYPES.filter(
  (o) => o.value !== 'PORTMAP_TCP' && o.value !== 'PORTMAP_UDP'
)

export const PORTMAP_OPERATION_TYPES = OPERATION_TYPES.filter(
  (o) => o.value === 'PORTMAP_TCP' || o.value === 'PORTMAP_UDP'
)

export function operationTypeLabel(type) {
  return OPERATION_TYPES.find((o) => o.value === type)?.label || type || t('common.emDash')
}

export function statusLabel(s) {
  return ({
    RUNNING: t('audit.format.statusRunning'),
    COMPLETED: t('audit.format.statusCompleted'),
    FAILED: t('audit.format.statusFailed'),
    INTERRUPTED: t('audit.format.statusInterrupted'),
    PURGED: t('audit.format.statusPurged')
  })[s] || s || t('common.emDash')
}

export function statusTagType(s) {
  return ({
    RUNNING: 'primary',
    COMPLETED: 'success',
    FAILED: 'danger',
    INTERRUPTED: 'warning',
    PURGED: 'info'
  })[s] || 'info'
}

export function eventTypeLabel(type) {
  return ({
    LIST: t('audit.format.eventList'),
    STAT: t('audit.format.eventStat'),
    MKDIR: t('audit.format.eventMkdir'),
    REMOVE: t('audit.format.eventRemove'),
    RENAME: t('audit.format.eventRename'),
    READ: t('audit.format.eventRead'),
    WRITE: t('audit.format.eventWrite'),
    TRANSFER_STATUS: t('audit.format.eventTransferStatus'),
    TRANSFER_COMMIT: t('audit.format.eventTransferCommit'),
    TRANSFER_ABORT: t('audit.format.eventTransferAbort'),
    TRANSFER_ERROR: t('audit.format.eventTransferError'),
    ROOTS: t('audit.format.eventRoots'),
    RUN: t('audit.format.eventRun'),
    COMMAND: t('audit.format.eventCommand'),
    ERROR: t('audit.format.eventError')
  })[type] || type || t('common.emDash')
}

export function recordingFormatLabel(f) {
  return ({
    asciinema: t('audit.format.recordingAsciinema'),
    guacamole: t('audit.format.recordingGuacamole')
  })[f] || f || ''
}

export { formatBytes, formatDateTime }

/** Elapsed time between occurredAt and endedAt; running operations show em dash. */
export function formatDuration(startIso, endIso) {
  if (!startIso || !endIso) return t('common.emDash')
  const start = new Date(startIso).getTime()
  const end = new Date(endIso).getTime()
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return t('common.emDash')
  const total = Math.round((end - start) / 1000)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const p = (n) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${p(m)}:${p(s)}` : `${m}:${p(s)}`
}

export function assetLabel(a) {
  const name = a.displayName || a.hostname || a.id
  return a.groupName ? `${name} · ${a.groupName}` : name
}

export function recordingPath(row) {
  return row?.detail?.recordingPath || ''
}

/** One-line summary of a detail object; long payload fields are never present. */
export function formatDetail(detail) {
  if (!detail || typeof detail !== 'object') return ''
  const parts = []
  if (detail.ephemeral && detail.initiator === 'opsctl') {
    parts.push(t('audit.format.ephemeralOpsctl'))
    if (detail.direction === 'opsctl_to_asset') {
      parts.push(t('audit.format.directionOpsctlToAsset'))
    } else if (detail.direction === 'asset_to_opsctl') {
      parts.push(t('audit.format.directionAssetToOpsctl'))
    }
  }
  if (detail.shellKind) parts.push(detail.shellKind)
  if (detail.command) parts.push(detail.command)
  if (detail.cwd) parts.push(`cwd ${detail.cwd}`)
  if (detail.exitCode != null) parts.push(`exit ${detail.exitCode}`)
  if (detail.durationMs != null) parts.push(`${detail.durationMs}ms`)
  if (detail.path) parts.push(detail.path)
  if (detail.to) parts.push(`→ ${detail.to}`)
  if (detail.bytes != null) parts.push(formatBytes(detail.bytes))
  if (detail.chunks != null && detail.chunks > 1) parts.push(t('audit.format.chunks', { n: detail.chunks }))
  if (detail.offset) parts.push(`offset ${detail.offset}`)
  if (detail.targetHost) {
    parts.push(`${detail.targetHost}${detail.targetPort != null ? ':' + detail.targetPort : ''}`)
  }
  if (detail.desktopUsername) parts.push(t('audit.format.desktopUser', { name: detail.desktopUsername }))
  if (detail.listenPort != null) parts.push(`listen:${detail.listenPort}`)
  if (detail.clientAddr) parts.push(`client:${detail.clientAddr}`)
  if (detail.bytesIn != null || detail.bytesOut != null) {
    parts.push(`↓${formatBytes(detail.bytesIn)} ↑${formatBytes(detail.bytesOut)}`)
  }
  if (detail.error) parts.push(detail.error)
  return parts.join(' · ')
}
