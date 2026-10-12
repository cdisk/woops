/** Compact alert age for tags: 3d+ → Nd; else 1d5h / 1h32m / 5m / <1m. */
export function formatAlertDuration(startedAt) {
  if (!startedAt) return ''
  const start = Date.parse(startedAt)
  if (Number.isNaN(start)) return ''
  const sec = Math.max(0, Math.floor((Date.now() - start) / 1000))
  const day = Math.floor(sec / 86400)
  const hour = Math.floor((sec % 86400) / 3600)
  const min = Math.floor((sec % 3600) / 60)
  if (day >= 3) return `${day}d`
  if (day >= 1) return `${day}d${hour}h`
  if (hour >= 1) return `${hour}h${min}m`
  if (min >= 1) return `${min}m`
  return '<1m'
}
