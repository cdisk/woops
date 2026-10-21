import api from '../../shared/api'
import { openExecWs } from '../../features/exec/execWs'
import { t } from '../../i18n'
import { isLegacyWindows, isWindows } from '../../session/assetOs'

export const AGENT_UPDATE_VERIFY_MS = 120_000
const AGENT_UPDATE_POLL_MS = 1000

export function installCommandFor(asset, prep) {
  if (!prep) return ''
  if (isWindows(asset)) {
    return isLegacyWindows(asset) ? (prep.cmd || prep.powershell || '') : (prep.powershell || '')
  }
  return prep.curl || ''
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function errorMessage(error) {
  return error?.response?.data?.error || error?.message || t('assets.updateFailed')
}

/**
 * Prepare the install command, run it on an exec ticket, then poll until the
 * agent is back online with a new version (or the 120s budget runs out).
 *
 * @returns {Promise<{status: string, started: boolean, asset?: object, message?: string, fromVersion?: string}>}
 * status: ok | unchanged | timeout | error | skipped | aborted
 */
export async function runAgentUpdate(asset, handlers = {}) {
  const {
    onOutput = () => {},
    onStatus = () => {},
    onPhase = () => {},
    onRemain = () => {},
    onAsset = () => {},
    shouldAbort = () => false,
    bindClose = () => {}
  } = handlers

  if (shouldAbort()) return { status: 'aborted', started: false }
  if (!asset?.online) return { status: 'skipped', reason: 'offline', started: false }
  if (!asset?.groupId) return { status: 'skipped', reason: 'ungrouped', started: false }

  onPhase('info')
  onStatus(t('assets.prepareInstallCmd'))
  let prep
  try {
    const res = await api.post(`/assets/${asset.id}/agent-update`)
    prep = res.data
  } catch (error) {
    const message = errorMessage(error)
    onOutput(`\n[error] ${message}\n`)
    onPhase('error')
    onStatus(t('assets.failWithMsg', { msg: message }))
    return { status: 'error', started: false, message }
  }
  if (shouldAbort()) return { status: 'aborted', started: false }

  const command = installCommandFor(asset, prep)
  if (!command) {
    const message = t('assets.noInstallCmd')
    onOutput(`\n[error] ${message}\n`)
    onPhase('error')
    onStatus(t('assets.failWithMsg', { msg: message }))
    return { status: 'error', started: false, message }
  }
  const fromVersion = String(prep.fromAgentVersion || asset.agentVersion || '')
  onOutput(`# from Agent ${fromVersion || t('common.emDash')}\n# ${command}\n\n`)
  onStatus(t('assets.applyExecSession'))

  let ticket
  try {
    const res = await api.post('/sessions/ticket', { assetId: asset.id, protocol: 'exec' })
    ticket = res.data
  } catch (error) {
    const message = errorMessage(error)
    onOutput(`\n[error] ${message}\n`)
    onPhase('error')
    onStatus(t('assets.failWithMsg', { msg: message }))
    return { status: 'error', started: false, message }
  }
  if (!ticket?.browserWs) {
    const message = t('assets.ticketNoBrowserWs')
    onOutput(`\n[error] ${message}\n`)
    onPhase('error')
    onStatus(t('assets.failWithMsg', { msg: message }))
    return { status: 'error', started: false, message }
  }
  if (shouldAbort()) return { status: 'aborted', started: false }

  onPhase('downloading')
  onStatus(t('assets.runningInstall'))
  const handle = openExecWs(ticket.browserWs, command, {
    onOutput,
    onStatus: (text) => onStatus(text),
    hasLog: () => true
  })
  bindClose(() => handle.close())
  try {
    await handle.done
  } finally {
    handle.close()
  }
  if (shouldAbort()) return { status: 'aborted', started: true, fromVersion }

  const polled = await pollAgentUpdate(asset.id, fromVersion, {
    onOutput, onStatus, onPhase, onRemain, onAsset, shouldAbort
  })
  return { ...polled, started: true, fromVersion }
}

async function pollAgentUpdate(assetId, fromVersion, handlers) {
  const baseline = String(fromVersion || '').trim()
  const started = Date.now()
  handlers.onPhase('verifying')
  handlers.onStatus(t('assets.verifyingStatus'))
  handlers.onOutput('\n[verify] polling asset status every 1s…\n')
  handlers.onRemain(Math.ceil(AGENT_UPDATE_VERIFY_MS / 1000))

  let lastOnline = null
  let lastVersion = ''
  let sawOffline = false
  let lastAsset = null
  let primed = false

  while (Date.now() - started <= AGENT_UPDATE_VERIFY_MS) {
    if (handlers.shouldAbort()) return { status: 'aborted', asset: lastAsset }
    try {
      const { data } = await api.get(`/assets/${assetId}`)
      if (handlers.shouldAbort()) return { status: 'aborted', asset: data }
      const ver = data.agentVersion || ''
      const online = !!data.online
      if (!primed) {
        lastOnline = online
        lastVersion = ver
        sawOffline = !online
        primed = true
      }
      if (!online) sawOffline = true
      const versionChanged = !!(baseline && ver && ver !== baseline)
        || (!baseline && !!ver && ver !== lastVersion)
      const changed = online !== lastOnline || ver !== lastVersion
      lastAsset = data
      if (changed) handlers.onAsset(data, { refreshList: true })
      lastOnline = online
      lastVersion = ver

      const elapsed = Math.round((Date.now() - started) / 1000)
      const remain = Math.max(0, Math.ceil((AGENT_UPDATE_VERIFY_MS - (Date.now() - started)) / 1000))
      handlers.onRemain(remain)
      handlers.onStatus(t('assets.statusWaitVersion', {
        online: online ? t('common.online') : t('common.offline'),
        ver: ver || t('common.emDash'),
        elapsed
      }))

      if (online && versionChanged) {
        handlers.onRemain(0)
        handlers.onPhase('success')
        handlers.onStatus(baseline
          ? t('assets.onlineWithPrev', { ver, baseline })
          : t('assets.onlineWithVer', { ver }))
        handlers.onOutput(`[ok] online agentVersion=${ver}${baseline ? ` (was ${baseline})` : ''}\n`)
        handlers.onAsset(data, { refreshList: true })
        return { status: 'ok', asset: data }
      }

      if (online && sawOffline && baseline && ver === baseline && elapsed >= 8) {
        handlers.onRemain(0)
        handlers.onPhase('warn')
        handlers.onStatus(t('assets.versionUnchangedStatus', { ver }))
        handlers.onOutput(`[warn] online again but agentVersion unchanged (${ver})\n`)
        handlers.onAsset(data, { refreshList: true })
        return { status: 'unchanged', asset: data }
      }

      if (Date.now() - started >= AGENT_UPDATE_VERIFY_MS) {
        return finishTimeout(data, { online, ver, baseline, handlers })
      }
    } catch (error) {
      const elapsed = Math.round((Date.now() - started) / 1000)
      const remain = Math.max(0, Math.ceil((AGENT_UPDATE_VERIFY_MS - (Date.now() - started)) / 1000))
      handlers.onRemain(remain)
      handlers.onStatus(t('assets.refreshRetry', { elapsed }))
      if (Date.now() - started >= AGENT_UPDATE_VERIFY_MS) {
        handlers.onRemain(0)
        handlers.onOutput(`[error] verify poll failed: ${error?.message || error}\n`)
        handlers.onPhase('error')
        handlers.onStatus(t('assets.refreshTimeout'))
        return { status: 'timeout', asset: lastAsset, message: t('assets.refreshTimeout') }
      }
    }
    const wait = Math.min(AGENT_UPDATE_POLL_MS, AGENT_UPDATE_VERIFY_MS - (Date.now() - started))
    if (wait > 0) await sleep(wait)
  }
  return finishTimeout(lastAsset, {
    online: !!lastAsset?.online,
    ver: lastAsset?.agentVersion || '',
    baseline,
    handlers
  })
}

function finishTimeout(asset, { online, ver, baseline, handlers }) {
  handlers.onRemain(0)
  if (online && baseline && ver && ver !== baseline) {
    handlers.onPhase('success')
    handlers.onStatus(t('assets.onlineWithVer', { ver }))
    if (asset) handlers.onAsset(asset, { refreshList: true })
    return { status: 'ok', asset }
  }
  if (online) {
    handlers.onPhase('warn')
    handlers.onStatus(t('assets.verifyTimeoutOnline', {
      ver: ver || t('common.emDash'),
      baseline: baseline || t('common.emDash')
    }))
    if (asset) handlers.onAsset(asset, { refreshList: true })
    return { status: 'timeout', asset }
  }
  handlers.onPhase('error')
  handlers.onStatus(t('assets.verifyTimeoutOffline', { baseline: baseline || t('common.emDash') }))
  if (asset) handlers.onAsset(asset, { refreshList: true })
  return { status: 'timeout', asset }
}
