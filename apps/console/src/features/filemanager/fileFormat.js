/** Display helpers shared by the file manager page and the upload dialog. */

export function formatSize(n) {
  if (n == null || Number.isNaN(Number(n))) return '-'
  const v = Number(n)
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`
  if (v < 1024 * 1024 * 1024) return `${(v / (1024 * 1024)).toFixed(1)} MB`
  return `${(v / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

export function formatSpeed(n) {
  if (!n || n < 0) return '-'
  return `${formatSize(n < 1024 ? Math.round(n) : n)}/s`
}

export function formatDuration(ms) {
  const sec = Math.max(0, Math.round(ms / 1000))
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (h > 0) return `${h}h ${m}m`
  return m <= 0 ? `${s}s` : `${m}m ${s}s`
}

export function formatMtime(m) {
  if (m == null || m === '') return '-'
  if (typeof m === 'number') {
    const ms = m < 1e12 ? m * 1000 : m
    return new Date(ms).toLocaleString()
  }
  return String(m)
}
