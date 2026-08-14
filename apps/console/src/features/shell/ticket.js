import api from '../../shared/api'

export function shellProtocol(kind) {
  const k = String(kind || 'bash').toLowerCase()
  return k === 'powershell' ? 'shell_powershell' : 'shell_bash'
}

export async function requestShellTicket(assetId, kind) {
  const { data } = await api.post('/sessions/ticket', {
    assetId,
    protocol: shellProtocol(kind)
  })
  return data
}
