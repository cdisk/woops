/**
 * Same-origin WS via Console HTTPS (/ws → Gateway).
 * Avoids a second self-signed trust prompt for :9200.
 */
export function rewriteWs(url) {
  return String(url || '').replace(
    /^wss?:\/\/[^/]+/,
    `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}`
  )
}

/** Close tab if opened via window.open; otherwise return to asset list. */
export function closeSessionTab() {
  if (window.opener) {
    window.close()
    return
  }
  window.location.href = '/assets'
}
