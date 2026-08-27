import api from '../../shared/api'

export function shellProtocol(kind) {
  const k = String(kind || 'bash').toLowerCase()
  if (k === 'powershell') return 'shell_powershell'
  if (k === 'cmd') return 'shell_cmd'
  return 'shell_bash'
}

export async function requestShellTicket(assetId, kind) {
  const { data } = await api.post('/sessions/ticket', {
    assetId,
    protocol: shellProtocol(kind)
  })
  return data
}
