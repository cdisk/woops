/** True when the asset is treated as Windows (RDP / PowerShell). */
export function isWindows(row) {
  if (!row) return false
  if (row.accessProtocol === 'rdp') return true
  const os = (row.os || '').toLowerCase()
  return os === 'windows' || os.includes('windows')
}

/** Win7 / Server 2008–2012 or legacy agent build: use cmd + WinPTY instead of PowerShell/ConPTY. */
export function isLegacyWindows(row) {
  if (!isWindows(row)) return false
  const ver = String(row.agentVersion || '').toLowerCase()
  if (ver.endsWith('-legacy')) return true
  const os = (row.os || '').toLowerCase().replace(/\s+/g, '')
  if (os.includes('windows7') || os.includes('windowsvista') || os.includes('windowsxp')) return true
  if (os.includes('server2008') || os.includes('server2012')) return true
  return false
}
