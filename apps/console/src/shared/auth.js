import api from './api'

const CAP_KEY = 'capabilities'

export function getRole() {
  return localStorage.getItem('role') || ''
}

export function getUsername() {
  return localStorage.getItem('username') || ''
}

export function getNickname() {
  return localStorage.getItem('nickname') || ''
}

/** Prefer nickname for UI chrome; fall back to login username. */
export function getDisplayName() {
  return getNickname() || getUsername() || ''
}

export function isSuperAdmin() {
  return getRole() === 'SUPER_ADMIN'
}

export function isAdmin() {
  const r = getRole()
  return r === 'ADMIN' || r === 'SUPER_ADMIN'
}

export function canManageInventory() {
  return isAdmin()
}

export function canManageUsers() {
  return isAdmin()
}

export function canManageAlertRules() {
  return isSuperAdmin()
}

export function setSession({ token, username, nickname, role, capabilities }) {
  if (token) localStorage.setItem('token', token)
  if (username) localStorage.setItem('username', username)
  if (nickname !== undefined && nickname !== null) {
    localStorage.setItem('nickname', nickname)
  }
  if (role) localStorage.setItem('role', role)
  if (capabilities) localStorage.setItem(CAP_KEY, JSON.stringify(capabilities))
}

export function clearSession() {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  localStorage.removeItem('nickname')
  localStorage.removeItem('role')
  localStorage.removeItem(CAP_KEY)
}

export async function refreshMe() {
  const { data } = await api.get('/auth/me')
  setSession({
    username: data.username,
    nickname: data.nickname || '',
    role: data.role,
    capabilities: data.capabilities || {}
  })
  return data
}
