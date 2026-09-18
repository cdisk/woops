<template>
  <div class="asset-detail" v-loading="loading">
    <div class="top-bar">
      <div class="top-left">
        <el-input
          v-model="form.displayName"
          class="name-input"
          :placeholder="t('assets.displayNamePlaceholder')"
          @keyup.enter="saveBasic"
        />
        <el-tag :type="asset?.online ? 'success' : 'info'" size="small">
          {{ asset?.online ? t('common.online') : t('common.offline') }}
        </el-tag>
        <el-tag v-if="isWindows" size="small" type="warning">WIN</el-tag>
        <el-tag v-else size="small" type="success">LINUX</el-tag>
      </div>
      <div class="top-right">
        <div class="actions-cluster">
          <el-button @click="goOperationAudit">
            <IconClipboardList :size="16" stroke="1.75" />
            {{ t('assets.operationsAudit') }}
          </el-button>
          <el-button @click="goControlAudit">{{ t('assets.controlAudit') }}</el-button>
          <el-button @click="goAssetEvents">{{ t('assets.assetEvents') }}</el-button>
          <el-button
            plain
            type="warning"
            :loading="updating"
            @click="oneClickUpdate"
          >
            <IconRefresh :size="16" stroke="1.75" />
            {{ t('assets.oneClickUpdate') }}
          </el-button>
          <el-button
            v-if="asset?.canDelete"
            type="danger"
            plain
            @click="removeAsset"
          >
            <IconTrash :size="16" stroke="1.75" />
            {{ t('assets.deleteAsset') }}
          </el-button>
        </div>
        <div class="save-slot">
          <el-button type="primary" :loading="saving" @click="saveBasic">
            <IconDeviceFloppy :size="16" stroke="1.75" />
            {{ t('common.save') }}
          </el-button>
        </div>
      </div>
    </div>

    <p class="meta">{{ metaLine }}</p>

    <section class="remark-section">
      <h3>{{ t('common.remark') }}</h3>
      <el-input
        v-model="form.remark"
        type="textarea"
        :rows="3"
        maxlength="4096"
        show-word-limit
        :placeholder="t('assets.remarkPlaceholder')"
      />
    </section>

    <section class="cred-section">
      <h3>{{ t('assets.desktopCredentials') }}</h3>
      <el-form label-width="96px" class="cred-form" @submit.prevent>
        <el-form-item :label="t('common.group')">
          <el-tree-select
            v-model="form.groupId"
            :data="groupSelectData"
            check-strictly
            clearable
            filterable
            default-expand-all
            :placeholder="t('assets.groupPlaceholder')"
            :props="{ label: 'label', value: 'id', children: 'children' }"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item :label="isWindows ? t('assets.rdpPort') : t('assets.vncPort')">
          <el-input-number v-model="form.desktopPort" :min="1" :max="65535" controls-position="right" />
        </el-form-item>
        <el-form-item v-if="isWindows" :label="t('assets.rdpUser')">
          <el-input v-model="form.desktopUsername" />
        </el-form-item>
        <el-form-item :label="isWindows ? t('assets.rdpPassword') : t('assets.vncPassword')">
          <el-input
            v-model="form.desktopPassword"
            type="password"
            show-password
            :placeholder="form.hasDesktopPassword ? t('assets.passwordSetPlaceholder') : (isWindows ? t('assets.passwordRequiredWindows') : t('assets.passwordRequiredVnc'))"
          />
        </el-form-item>
        <template v-if="isWindows">
          <el-form-item :label="t('assets.rdpColorDepth')">
            <el-select v-model="form.desktopColorDepth" style="width: 100%">
              <el-option :value="8" :label="t('assets.colorDepth8')" />
              <el-option :value="16" :label="t('assets.colorDepth16')" />
              <el-option :value="24" :label="t('assets.colorDepth24')" />
              <el-option :value="32" :label="t('assets.colorDepth32')" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('assets.rdpQuality')">
            <el-select v-model="form.desktopRdpQuality" style="width: 100%">
              <el-option value="low" :label="t('assets.qualityLow')" />
              <el-option value="medium" :label="t('assets.qualityMedium')" />
              <el-option value="high" :label="t('assets.qualityHigh')" />
            </el-select>
          </el-form-item>
        </template>
      </el-form>
      <p class="hint">
        {{ t('assets.hintIp') }}
        <template v-if="isWindows">{{ t('assets.hintRdpQuality') }}</template>
      </p>
    </section>

    <div class="split">
      <section class="panel">
        <h3>{{ t('assets.portmapSection') }}</h3>
        <PortMapPanel v-if="assetId" :asset-id="assetId" :assets="asset ? [asset] : []" />
      </section>
      <section class="panel">
        <DeployTokenPanel v-if="assetId" :asset-id="assetId" />
      </section>
    </div>

    <el-dialog
      v-model="execDialogVisible"
      :title="t('assets.updateAgentTitle')"
      width="720px"
      align-center
      :close-on-click-modal="!execRunning"
      :close-on-press-escape="!execRunning"
      @closed="onExecDialogClosed"
    >
      <div class="exec-status-bar" :class="'is-' + execStatusKind">
        <template v-if="execStatusKind === 'verifying'">
          <div class="exec-status-main">
            <span class="exec-status-label">{{ t('assets.waitingAgentOnline') }}</span>
            <span class="exec-status-countdown">{{ t('assets.remainSec', { n: execVerifyRemainSec }) }}</span>
          </div>
          <div class="exec-status-meta">{{ execStatusText }}</div>
        </template>
        <template v-else-if="execStatusKind === 'success'">
          <div class="exec-status-main">
            <span class="exec-status-label">{{ t('assets.updateSuccess') }}</span>
          </div>
          <div class="exec-status-meta">{{ execStatusText }}</div>
        </template>
        <template v-else>
          <div class="exec-status-main">
            <span class="exec-status-label">{{ execStatusText || t('common.preparing') }}</span>
          </div>
        </template>
      </div>
      <pre ref="execLogRef" class="exec-log">{{ execLog || t('common.waitingOutput') }}</pre>
      <template #footer>
        <el-button :disabled="execRunning" @click="execDialogVisible = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { IconClipboardList, IconDeviceFloppy, IconRefresh, IconTrash } from '@tabler/icons-vue'
import api from '../../shared/api'
import PortMapPanel from '../../features/portmap/PortMapPanel.vue'
import DeployTokenPanel from './DeployTokenPanel.vue'
import { openExecWs } from '../../features/exec/execWs'
import {
  ASSET_EVENTS_AUDIT_PATH,
  CONTROL_AUDIT_PATH,
  OPERATIONS_AUDIT_PATH
} from '../../features/audit/routes'
import { isLegacyWindows, isWindows as assetIsWindows } from '../../session/assetOs'

const props = defineProps({
  assetId: { type: String, required: true }
})

const emit = defineEmits(['close', 'changed'])

const { t } = useI18n()
const router = useRouter()

const loading = ref(false)
const saving = ref(false)
const updating = ref(false)
const asset = ref(null)
const groupTree = ref([])

const execDialogVisible = ref(false)
const execRunning = ref(false)
const execLog = ref('')
const execStatusText = ref('')
/** info | verifying | success | warn | error */
const execStatusKind = ref('info')
const execVerifyRemainSec = ref(0)
const execLogRef = ref(null)
let execHandle = null
let execVerifyTimer = null
let execVerifyInFlight = false
let execVerifyGeneration = 0

const AGENT_UPDATE_VERIFY_MS = 120_000
const AGENT_UPDATE_VERIFY_INTERVAL_MS = 1000

function setExecStatus(kind, text) {
  execStatusKind.value = kind || 'info'
  if (text != null) execStatusText.value = text
}

const form = reactive({
  displayName: '',
  remark: '',
  groupId: null,
  desktopPort: 5900,
  desktopUsername: '',
  desktopPassword: '',
  hasDesktopPassword: false,
  desktopColorDepth: 16,
  desktopRdpQuality: 'low'
})

const isWindows = computed(() => assetIsWindows(asset.value))
const isLegacyWin = computed(() => isLegacyWindows(asset.value))

const groupSelectData = computed(() => mapGroups(groupTree.value))

const metaLine = computed(() => {
  const a = asset.value
  if (!a) return ''
  const parts = []
  if (a.hostname) parts.push(a.hostname)
  const ips = []
  if (a.publicIp) ips.push(a.publicIp)
  if (a.privateIp) {
    for (const ip of String(a.privateIp).split(/[,\s]+/).filter(Boolean)) {
      if (!ips.includes(ip)) ips.push(ip)
    }
  }
  if (ips.length) parts.push(ips.join(' / '))
  if (a.groupName) parts.push(t('assets.groupLabel', { name: a.groupName }))
  else parts.push(t('common.ungrouped'))
  if (a.agentVersion) parts.push(`Agent ${a.agentVersion}`)
  if (a.os) parts.push(a.os)
  return parts.join(' · ')
})

function mapGroups(nodes) {
  return (nodes || []).map((n) => ({
    id: n.id,
    label: n.name,
    children: mapGroups(n.children || [])
  }))
}

function applyAsset(data) {
  asset.value = data
  form.displayName = data.displayName || ''
  form.remark = data.remark || ''
  form.groupId = data.groupId || null
  form.desktopPort = data.desktopPort || (isWindows.value ? 3389 : 5900)
  form.desktopUsername = data.desktopUsername || ''
  form.desktopPassword = ''
  form.hasDesktopPassword = !!data.hasDesktopPassword
  form.desktopColorDepth = [8, 16, 24, 32].includes(data.desktopColorDepth) ? data.desktopColorDepth : 16
  form.desktopRdpQuality = ['low', 'medium', 'high'].includes(data.desktopRdpQuality) ? data.desktopRdpQuality : 'low'
}

async function load() {
  if (!props.assetId) return
  loading.value = true
  try {
    const [{ data: a }, { data: g }] = await Promise.all([
      api.get(`/assets/${props.assetId}`),
      api.get('/groups')
    ])
    applyAsset(a)
    groupTree.value = g || []
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveBasic() {
  if (!form.displayName?.trim()) {
    ElMessage.warning(t('assets.displayNameRequired'))
    return
  }
  saving.value = true
  try {
    const body = {
      displayName: form.displayName.trim(),
      remark: form.remark,
      groupId: form.groupId || null,
      updateGroup: true,
      desktopPort: form.desktopPort,
      desktopUsername: form.desktopUsername,
      desktopColorDepth: form.desktopColorDepth,
      desktopRdpQuality: form.desktopRdpQuality
    }
    if (form.desktopPassword) body.desktopPassword = form.desktopPassword
    const { data } = await api.patch(`/assets/${props.assetId}`, body)
    applyAsset(data)
    ElMessage.success(t('common.saveSuccess'))
    emit('changed')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function removeAsset() {
  const label = (form.displayName || asset.value?.hostname || props.assetId || '').trim()
  const escaped = label
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
  try {
    await ElMessageBox.prompt(
      t('assets.deleteConfirmHtml', { name: escaped }),
      t('assets.deleteTitle'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
        dangerouslyUseHTMLString: true,
        inputPlaceholder: t('assets.inputPlaceholder', { name: label }),
        inputValidator: (value) => {
          if ((value || '').trim() !== label) return t('assets.nameMismatch')
          return true
        }
      }
    )
  } catch {
    return
  }
  try {
    await api.delete(`/assets/${props.assetId}`)
    ElMessage.success(t('common.deleteSuccess'))
    emit('changed')
    emit('close')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.deleteFailed'))
  }
}

function goOperationAudit() {
  emit('close')
  router.push({ path: OPERATIONS_AUDIT_PATH, query: { assetId: props.assetId } })
}

function goControlAudit() {
  emit('close')
  router.push({ path: CONTROL_AUDIT_PATH, query: { assetId: props.assetId } })
}

function goAssetEvents() {
  emit('close')
  router.push({ path: ASSET_EVENTS_AUDIT_PATH, query: { assetId: props.assetId } })
}

function appendExecLog(chunk) {
  execLog.value += chunk
  nextTick(() => {
    const el = execLogRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

function closeExecWs() {
  if (execHandle) {
    execHandle.close()
    execHandle = null
  }
}

function stopAgentUpdateVerify() {
  if (execVerifyTimer != null) {
    clearInterval(execVerifyTimer)
    execVerifyTimer = null
  }
  execVerifyInFlight = false
}

function onExecDialogClosed() {
  execVerifyGeneration += 1
  stopAgentUpdateVerify()
  closeExecWs()
  execRunning.value = false
  execStatusKind.value = 'info'
  execVerifyRemainSec.value = 0
}

/**
 * After exec finishes (or disconnects on Agent restart), poll asset every 1s:
 * refresh online + agentVersion on this page (and list via changed) until
 * online with a new version, or timeout.
 */
function waitAgentUpdateResult(fromVer) {
  const baseline = String(fromVer || '').trim()
  stopAgentUpdateVerify()
  const gen = ++execVerifyGeneration
  execVerifyRemainSec.value = Math.ceil(AGENT_UPDATE_VERIFY_MS / 1000)
  setExecStatus('verifying', t('assets.verifyingStatus'))
  appendExecLog('\n[verify] polling asset status every 1s…\n')

  return new Promise((resolve) => {
    const started = Date.now()
    let lastOnline = asset.value?.online
    let lastVersion = asset.value?.agentVersion || ''
    let sawOffline = !lastOnline
    let settled = false

    const finish = (result) => {
      if (settled) return
      settled = true
      stopAgentUpdateVerify()
      resolve(result)
    }

    const tick = async () => {
      if (settled) return
      if (gen !== execVerifyGeneration || !execDialogVisible.value) {
        finish('aborted')
        return
      }
      if (execVerifyInFlight) return
      execVerifyInFlight = true
      try {
        const remain = Math.max(0, Math.ceil((AGENT_UPDATE_VERIFY_MS - (Date.now() - started)) / 1000))
        execVerifyRemainSec.value = remain

        const { data: a } = await api.get(`/assets/${props.assetId}`)
        if (settled || gen !== execVerifyGeneration || !execDialogVisible.value) {
          finish('aborted')
          return
        }
        const ver = a.agentVersion || ''
        const online = !!a.online
        if (!online) sawOffline = true

        const versionChanged = !!(baseline && ver && ver !== baseline)
          || (!baseline && !!ver && ver !== lastVersion)
        const onlineChanged = online !== lastOnline
        const verFieldChanged = ver !== lastVersion
        if (onlineChanged || verFieldChanged) {
          applyAsset(a)
          emit('changed')
        } else {
          asset.value = { ...asset.value, ...a, online, agentVersion: ver }
        }
        lastOnline = online
        lastVersion = ver

        const elapsed = Math.round((Date.now() - started) / 1000)
        setExecStatus(
          'verifying',
          t('assets.statusWaitVersion', {
            online: online ? t('common.online') : t('common.offline'),
            ver: ver || t('common.emDash'),
            elapsed
          })
        )

        if (online && versionChanged) {
          execVerifyRemainSec.value = 0
          setExecStatus(
            'success',
            baseline
              ? t('assets.onlineWithPrev', { ver, baseline })
              : t('assets.onlineWithVer', { ver })
          )
          appendExecLog(`[ok] online agentVersion=${ver}${baseline ? ` (was ${baseline})` : ''}\n`)
          ElMessage.success(t('assets.agentUpdated', { ver }))
          applyAsset(a)
          emit('changed')
          finish('ok')
          return
        }

        if (online && sawOffline && baseline && ver === baseline && elapsed >= 8) {
          execVerifyRemainSec.value = 0
          setExecStatus('warn', t('assets.versionUnchangedStatus', { ver }))
          appendExecLog(`[warn] online again but agentVersion unchanged (${ver})\n`)
          ElMessage.warning(t('assets.versionUnchangedWarn'))
          applyAsset(a)
          emit('changed')
          finish('unchanged')
          return
        }

        if (Date.now() - started >= AGENT_UPDATE_VERIFY_MS) {
          execVerifyRemainSec.value = 0
          if (online && versionChanged) {
            setExecStatus('success', t('assets.onlineWithVer', { ver }))
            ElMessage.success(t('assets.agentUpdated', { ver }))
            applyAsset(a)
            emit('changed')
            finish('ok')
            return
          }
          if (online) {
            setExecStatus(
              'warn',
              t('assets.verifyTimeoutOnline', {
                ver: ver || t('common.emDash'),
                baseline: baseline || t('common.emDash')
              })
            )
            ElMessage.warning(t('assets.verifyTimeoutWarn'))
          } else {
            setExecStatus(
              'error',
              t('assets.verifyTimeoutOffline', { baseline: baseline || t('common.emDash') })
            )
            ElMessage.warning(t('assets.agentOnlineTimeout'))
          }
          applyAsset(a)
          emit('changed')
          finish('timeout')
        }
      } catch (e) {
        const elapsed = Math.round((Date.now() - started) / 1000)
        const remain = Math.max(0, Math.ceil((AGENT_UPDATE_VERIFY_MS - (Date.now() - started)) / 1000))
        execVerifyRemainSec.value = remain
        setExecStatus('verifying', t('assets.refreshRetry', { elapsed }))
        if (Date.now() - started >= AGENT_UPDATE_VERIFY_MS) {
          execVerifyRemainSec.value = 0
          appendExecLog(`[error] verify poll failed: ${e?.message || e}\n`)
          setExecStatus('error', t('assets.refreshTimeout'))
          finish('timeout')
        }
      } finally {
        execVerifyInFlight = false
      }
    }

    execVerifyTimer = setInterval(tick, AGENT_UPDATE_VERIFY_INTERVAL_MS)
    tick()
  })
}

async function oneClickUpdate() {
  if (!asset.value?.online) {
    ElMessage.warning(t('assets.offlineCannotUpdate'))
    return
  }
  if (!asset.value?.groupId) {
    ElMessage.warning(t('assets.ungroupedCannotUpdate'))
    return
  }
  const savedGroup = asset.value.groupId || null
  const formGroup = form.groupId || null
  if (String(savedGroup) !== String(formGroup)) {
    ElMessage.warning(t('assets.unsavedGroupCannotUpdate'))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('assets.updateConfirm'),
      t('assets.updateAgentTitle'),
      { confirmButtonText: t('assets.startUpdate'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }

  updating.value = true
  execLog.value = ''
  execVerifyRemainSec.value = 0
  setExecStatus('info', t('assets.prepareInstallCmd'))
  execDialogVisible.value = true
  execRunning.value = true
  try {
    const { data: prep } = await api.post(`/assets/${props.assetId}/agent-update`)
    const command = isWindows.value
      ? (isLegacyWin.value ? (prep.cmd || prep.powershell || '') : (prep.powershell || ''))
      : (prep.curl || '')
    if (!command) {
      throw new Error(t('assets.noInstallCmd'))
    }
    const fromVer = prep.fromAgentVersion || asset.value.agentVersion || ''
    appendExecLog(`# from Agent ${fromVer || t('common.emDash')}\n# ${command}\n\n`)
    setExecStatus('info', t('assets.applyExecSession'))

    const { data: ticket } = await api.post('/sessions/ticket', {
      assetId: props.assetId,
      protocol: 'exec'
    })
    if (!ticket?.browserWs) {
      throw new Error(t('assets.ticketNoBrowserWs'))
    }

    setExecStatus('info', t('assets.runningInstall'))
    closeExecWs()
    execHandle = openExecWs(ticket.browserWs, command, {
      onOutput: appendExecLog,
      onStatus: (text) => { setExecStatus('info', text) },
      hasLog: () => !!execLog.value.trim()
    })
    await execHandle.done
    closeExecWs()
    // Install often kills the exec session on Agent restart — still verify online + version.
    await waitAgentUpdateResult(fromVer)
    execRunning.value = false
  } catch (e) {
    const msg = e?.response?.data?.error || e?.message || t('assets.updateFailed')
    appendExecLog(`\n[error] ${msg}\n`)
    setExecStatus('error', t('assets.failWithMsg', { msg }))
    ElMessage.error(msg)
    execRunning.value = false
  } finally {
    updating.value = false
  }
}

watch(() => props.assetId, load)
onMounted(load)
onBeforeUnmount(() => {
  execVerifyGeneration += 1
  stopAgentUpdateVerify()
  closeExecWs()
})
</script>

<style scoped>
.asset-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: calc(92vh - 80px);
  overflow: auto;
  padding-right: 4px;
}
.top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.top-left, .top-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.top-right {
  margin-left: auto;
}
.actions-cluster {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.save-slot {
  display: flex;
  align-items: center;
  margin-left: 12px;
  padding-left: 12px;
  border-left: 1px solid #e5e7eb;
}
.top-left :deep(.el-button),
.top-right :deep(.el-button) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.name-input { width: min(360px, 50vw); }
.name-input :deep(.el-input__inner) {
  font-size: 18px;
  font-weight: 600;
}
.meta {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}
.remark-section,
.cred-section {
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 12px 16px;
  background: #fff;
}
.remark-section h3,
.cred-section h3,
.panel h3 {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
}
.cred-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(220px, 1fr));
  gap: 0 24px;
  max-width: 820px;
}
.cred-form :deep(.el-form-item) { margin-bottom: 12px; }
.hint { color: #6b7280; font-size: 12px; margin: 0; }
.split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  min-height: 280px;
}
.panel {
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 12px;
  background: #fff;
  min-width: 0;
  overflow: auto;
}
.exec-status-bar {
  margin: 0 0 10px;
  padding: 10px 12px;
  border-radius: 6px;
  border: 1px solid #e5e7eb;
  background: #f8fafc;
}
.exec-status-main {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.exec-status-label {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.exec-status-countdown {
  font-size: 18px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: #dc2626;
  letter-spacing: 0.02em;
}
.exec-status-meta {
  margin-top: 4px;
  font-size: 12px;
  color: #64748b;
  line-height: 1.4;
}
.exec-status-bar.is-verifying {
  border-color: #fecaca;
  background: #fef2f2;
}
.exec-status-bar.is-verifying .exec-status-label {
  color: #b91c1c;
}
.exec-status-bar.is-success {
  border-color: #86efac;
  background: #f0fdf4;
}
.exec-status-bar.is-success .exec-status-label {
  color: #16a34a;
  font-size: 16px;
}
.exec-status-bar.is-success .exec-status-meta {
  color: #15803d;
}
.exec-status-bar.is-warn {
  border-color: #fde68a;
  background: #fffbeb;
}
.exec-status-bar.is-warn .exec-status-label {
  color: #b45309;
}
.exec-status-bar.is-error {
  border-color: #fecaca;
  background: #fef2f2;
}
.exec-status-bar.is-error .exec-status-label {
  color: #dc2626;
}
.exec-log {
  margin: 0;
  max-height: 420px;
  overflow: auto;
  padding: 12px;
  background: #0b1020;
  color: #e2e8f0;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
}
@media (max-width: 1100px) {
  .split { grid-template-columns: 1fr; }
  .cred-form { grid-template-columns: 1fr; }
  .save-slot {
    margin-left: 0;
    padding-left: 0;
    border-left: none;
  }
}
</style>
