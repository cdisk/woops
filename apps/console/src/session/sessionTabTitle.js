import { t } from '../i18n'

/** Default browser tab title for the main console (non-session pages). */
export const DEFAULT_DOCUMENT_TITLE = 'Woops'

/** First private IP from asset.privateIp (may be CSV). */
export function firstPrivateIp(privateIp) {
  if (!privateIp) return ''
  return String(privateIp).split(/[,\s]+/).filter(Boolean)[0] || ''
}

/**
 * Browser tab title for session/monitor windows so multiple tabs are distinguishable.
 * e.g. "Shell · web-01 · 10.0.0.5", "RDP · db · 192.168.1.10"
 */
export function formatSessionTabTitle(kindLabel, asset) {
  const kind = String(kindLabel || '').trim() || t('session.kindDefault')
  const name = asset?.displayName || asset?.hostname || ''
  const ip = firstPrivateIp(asset?.privateIp)
  const parts = [kind, name, ip].filter(Boolean)
  return parts.length ? parts.join(' · ') : DEFAULT_DOCUMENT_TITLE
}

export function setSessionDocumentTitle(kindLabel, asset) {
  document.title = formatSessionTabTitle(kindLabel, asset)
}
