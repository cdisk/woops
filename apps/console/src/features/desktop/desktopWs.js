/**
 * Guacamole.WebSocketTunnel.connect(data) always opens `tunnelURL + "?" + data`
 * with subprotocol "guacamole". Our ticket API returns `.../ws/desktop?ticket=...`,
 * so we must split base path vs query — otherwise connect() appends "?undefined"
 * and the ticket becomes invalid.
 */
import { rewriteWs } from '../../session/sessionWs'

export function splitDesktopWs(browserWs) {
  const rewritten = rewriteWs(browserWs)
  const q = rewritten.indexOf('?')
  if (q < 0) return { base: rewritten, connectData: '' }
  return {
    base: rewritten.slice(0, q),
    connectData: rewritten.slice(q + 1)
  }
}
