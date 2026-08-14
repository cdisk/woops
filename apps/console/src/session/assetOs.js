/** True when the asset is treated as Windows (RDP / PowerShell). */
export function isWindows(row) {
  if (!row) return false
  if (row.accessProtocol === 'rdp') return true
  const os = (row.os || '').toLowerCase()
  return os === 'windows' || os.includes('windows')
}
