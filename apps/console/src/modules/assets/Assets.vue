<template>
  <div class="assets-page">
    <div class="toolbar">
      <el-input
        v-model="searchQuery"
        clearable
        class="toolbar-search"
        :placeholder="t('assets.searchPlaceholder')"
      />
      <div class="toolbar-actions">
        <el-button @click="reloadAll">
          <IconRefresh :size="16" stroke="1.75" />
          {{ t('common.refresh') }}
        </el-button>
        <el-button v-if="canManageInventory()" type="primary" :loading="installLoading" @click="openInstall">
          <IconLink :size="16" stroke="1.75" />
          {{ t('assets.generateInstall') }}
        </el-button>
      </div>
    </div>

    <div class="explorer">
      <GroupTree ref="groupTreeRef" v-model="selectedKey" @change="onGroupChange" />

      <section class="list-pane">
        <div class="list-title">
          <span>{{ selectionLabel }}</span>
          <div class="list-title-right">
            <el-checkbox
              v-if="selectedKey !== ALL_KEY"
              v-model="showSubtree"
              @change="loadAssets"
            >{{ t('assets.showAll') }}</el-checkbox>
            <span class="list-count">{{ t('assets.count', { n: filteredAssets.length }) }}</span>
          </div>
        </div>
        <el-table :data="filteredAssets" v-loading="loading" stripe height="100%">
          <el-table-column prop="displayName" :label="t('common.name')" min-width="120" show-overflow-tooltip />
          <el-table-column prop="hostname" :label="t('assets.hostname')" min-width="120" show-overflow-tooltip />
          <el-table-column :label="t('assets.group')" width="120" show-overflow-tooltip>
            <template #default="{ row }">
              {{ row.groupId ? (row.groupName || t('common.emDash')) : t('common.ungrouped') }}
            </template>
          </el-table-column>
          <el-table-column :label="t('assets.onlineCol')" width="80">
            <template #default="{ row }">
              <el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? t('common.online') : t('common.offline') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="publicIp" :label="t('assets.publicIp')" width="140" show-overflow-tooltip />
          <el-table-column prop="privateIp" :label="t('assets.privateIp')" width="140" show-overflow-tooltip />
          <el-table-column :label="t('assets.os')" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <div class="os-cell">
                <span class="os-text">{{ row.os || t('common.emDash') }}</span>
                <el-tag size="small" class="proto" :type="isWindows(row) ? 'warning' : 'success'">
                  {{ isWindows(row) ? 'WIN' : 'LINUX' }}
                </el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="agentVersion" :label="t('assets.agentVersion')" width="120" show-overflow-tooltip>
            <template #default="{ row }">{{ row.agentVersion || t('common.emDash') }}</template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="220" fixed="right" align="right">
            <template #default="{ row }">
              <div class="row-actions">
                <el-button-group>
                  <el-button
                    v-for="action in sessionActionsFor(row)"
                    :key="action.id"
                    size="small"
                    :type="action.type"
                    :title="actionTitle(action, row)"
                    :disabled="action.requiresOnline && !row.online"
                    @click="runSessionAction(action, row)"
                  >
                    <component :is="action.icon" :size="16" stroke="1.75" />
                  </el-button>
                  <el-button size="small" :title="t('common.detail')" @click="openDetail(row)">
                    <IconSettings :size="16" stroke="1.75" />
                  </el-button>
                </el-button-group>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </div>

    <el-dialog
      v-model="detailVisible"
      :title="t('assets.settingsTitle')"
      width="92%"
      top="3vh"
      destroy-on-close
      append-to-body
      class="asset-detail-dialog"
      @closed="detailAssetId = ''"
    >
      <AssetDetail
        v-if="detailAssetId"
        :asset-id="detailAssetId"
        @close="detailVisible = false"
        @changed="loadAssets"
      />
    </el-dialog>

    <el-dialog v-model="installVisible" width="60%" @closed="onInstallClosed">
      <template #header>
        <span class="install-dialog-title">
          {{ t('assets.installCommands') }}
          <template v-if="installExpiresAt">
            （{{ installTitlePrefix }}<span class="install-countdown" :class="{ expired: installExpired }">{{ installTitleRemain }}</span>）
          </template>
        </span>
      </template>
      <p class="install-target">
        {{ t('assets.registerHint', { group: installGroupName }) }}
        <span v-if="installExpiresAt" class="install-ttl" :class="{ expired: installExpired }">
          <template v-if="installExpired">{{ t('assets.installExpired') }}</template>
          <template v-else>
            {{ t('assets.installRemain', { remain: formatRemain(installRemainMs) }) }}
          </template>
        </span>
      </p>
      <div v-if="installCode" class="install-block">
        <div class="install-label">{{ t('assets.installCode') }}</div>
        <p class="install-code-hint">{{ t('assets.installCodeHint') }}</p>
        <el-input type="textarea" :rows="2" v-model="installCode" readonly />
        <el-button size="small" class="copy-btn" @click="copyText(installCode)">
          {{ t('common.copy') }}
        </el-button>
      </div>
      <div class="install-block">
        <div class="install-label">Linux</div>
        <el-input type="textarea" :rows="3" v-model="installCurl" readonly />
        <el-button size="small" class="copy-btn" @click="copyText(installCurl)">
          <IconCopy :size="16" stroke="1.75" />
          {{ t('common.copy') }}
        </el-button>
      </div>
      <div class="install-block">
        <div class="install-label">Windows（PowerShell）</div>
        <el-input type="textarea" :rows="3" v-model="installPowershell" readonly />
        <el-button size="small" class="copy-btn" @click="copyText(installPowershell)">
          <IconCopy :size="16" stroke="1.75" />
          {{ t('common.copy') }}
        </el-button>
        <div class="install-curl-tip">
          <button type="button" class="install-curl-tip-toggle" @click="curlTipOpen = !curlTipOpen">
            <IconChevronDown :size="16" stroke="1.75" class="install-curl-tip-chevron" :class="{ open: curlTipOpen }" />
            <span>{{ t('assets.curlMissingHint') }}</span>
          </button>
          <div v-show="curlTipOpen" class="install-curl-tip-body">
            <p>{{ t('assets.curlInstallIntro') }}</p>
            <pre class="install-proxy-pre">{{ t('assets.curlInstallSteps') }}</pre>
          </div>
        </div>
      </div>
      <div class="install-block">
        <div class="install-label">{{ t('assets.installWindowsCmd') }}</div>
        <el-input type="textarea" :rows="3" v-model="installCmd" readonly />
        <el-button size="small" class="copy-btn" @click="copyText(installCmd)">
          <IconCopy :size="16" stroke="1.75" />
          {{ t('common.copy') }}
        </el-button>
      </div>
      <div class="install-proxy-note">
        <button type="button" class="install-curl-tip-toggle" @click="proxyTipOpen = !proxyTipOpen">
          <IconChevronDown :size="16" stroke="1.75" class="install-curl-tip-chevron" :class="{ open: proxyTipOpen }" />
          <span>{{ t('assets.proxyToggle') }}</span>
        </button>
        <div v-show="proxyTipOpen" class="install-curl-tip-body">
          <p>{{ t('assets.proxyBody1') }}</p>
          <ul>
            <li>{{ t('assets.proxyVars') }}</li>
            <li>{{ t('assets.proxyFormat') }}</li>
          </ul>
          <p class="install-proxy-examples">{{ t('assets.proxyLinuxExample') }}</p>
          <pre class="install-proxy-pre">export https_proxy='http://proxyuser:changeme@10.0.0.1:3128'
{{ t('assets.proxyLinuxComment') }}</pre>
          <p class="install-proxy-examples">{{ t('assets.proxyWinExample') }}</p>
          <pre class="install-proxy-pre">$env:https_proxy = 'http://proxyuser:changeme@10.0.0.1:3128'
{{ t('assets.proxyWinComment') }}</pre>
          <p class="install-proxy-examples">{{ t('assets.proxyWinCmdExample') }}</p>
          <pre class="install-proxy-pre">set "HTTPS_PROXY=http://proxyuser:changeme@10.0.0.1:3128"
set "https_proxy=http://proxyuser:changeme@10.0.0.1:3128"
{{ t('assets.proxyWinCmdComment') }}</pre>
          <p class="install-proxy-foot">{{ t('assets.proxyFoot') }}</p>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  IconChevronDown,
  IconCopy,
  IconLink,
  IconRefresh,
  IconSettings
} from '@tabler/icons-vue'
import api from '../../shared/api'
import { canManageInventory } from '../../shared/auth'
import { assetSessionActions } from '../../features/registry'
import { isWindows } from '../../session/assetOs'
import GroupTree, { ALL_KEY } from './GroupTree.vue'
import AssetDetail from './AssetDetail.vue'

const { t, locale } = useI18n()

const assets = ref([])
const searchQuery = ref('')
const loading = ref(false)
const selectedKey = ref(ALL_KEY)
const selectedGroupName = ref('')
const showSubtree = ref(false)
const groupTreeRef = ref(null)

const detailVisible = ref(false)
const detailAssetId = ref('')

const installLoading = ref(false)
const installVisible = ref(false)
const installGroupName = ref('')
const installCode = ref('')
const installCurl = ref('')
const installPowershell = ref('')
const installCmd = ref('')
const installExpiresAt = ref(null)
const installNow = ref(Date.now())
const curlTipOpen = ref(false)
const proxyTipOpen = ref(false)
let installCountdownTimer = null
let assetsPollTimer = null

const installRemainMs = computed(() => {
  if (!installExpiresAt.value) return 0
  return Math.max(0, installExpiresAt.value - installNow.value)
})
const installExpired = computed(() => !!installExpiresAt.value && installRemainMs.value <= 0)
const installTitleRemain = computed(() => {
  if (installExpired.value) return t('assets.expired')
  return formatRemain(installRemainMs.value)
})
const installTitlePrefix = computed(() => (installExpired.value ? '' : t('assets.remainPrefix')))

function formatRemain(ms) {
  const totalSec = Math.ceil(ms / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

function startInstallCountdown() {
  stopInstallCountdown()
  installNow.value = Date.now()
  installCountdownTimer = setInterval(() => {
    installNow.value = Date.now()
  }, 1000)
}

function stopInstallCountdown() {
  if (installCountdownTimer) {
    clearInterval(installCountdownTimer)
    installCountdownTimer = null
  }
}

function onInstallClosed() {
  stopInstallCountdown()
  installExpiresAt.value = null
  installCode.value = ''
}

const selectionLabel = computed(() => {
  if (selectedKey.value === ALL_KEY) return t('assets.allAssets')
  if (!selectedGroupName.value) return t('assets.title')
  return showSubtree.value
    ? t('assets.groupTitleWithChildren', { name: selectedGroupName.value })
    : t('assets.groupTitle', { name: selectedGroupName.value })
})

const filteredAssets = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return assets.value
  return assets.value.filter((a) =>
    [a.displayName, a.hostname, a.publicIp, a.privateIp]
      .some((v) => String(v || '').toLowerCase().includes(q))
  )
})

function syncSelectedGroupName() {
  if (selectedKey.value === ALL_KEY) {
    selectedGroupName.value = ''
    return
  }
  const node = groupTreeRef.value?.findGroup?.(selectedKey.value)
  selectedGroupName.value = node?.name || node?.label || ''
}

function onGroupChange() {
  syncSelectedGroupName()
  loadAssets()
}

function sessionActionsFor(row) {
  return assetSessionActions.filter((a) => !a.match || a.match(row))
}

function actionTitle(action, row) {
  return typeof action.title === 'function' ? action.title(row) : action.title
}

async function runSessionAction(action, row) {
  await action.open(row, { openDetail })
}

function openDetail(row) {
  detailAssetId.value = String(row.id)
  detailVisible.value = true
}

async function loadAssets() {
  loading.value = true
  try {
    const params = {}
    if (selectedKey.value !== ALL_KEY) {
      params.groupId = selectedKey.value
      if (showSubtree.value) params.includeSubtree = true
    }
    const { data } = await api.get('/assets', { params })
    assets.value = sortAssets(data || [])
  } catch (e) {
    assets.value = []
    const err = e?.response?.data?.error
    // Leave page / logout: stop spamming toasts from a stale poll.
    if (e?.response?.status === 401 || err === 'unauthorized' || err === 'forbidden') {
      return
    }
    ElMessage.error(err || t('assets.loadFailed'))
  } finally {
    loading.value = false
  }
}

function localeTag() {
  return locale.value === 'zh' ? 'zh-CN' : 'en'
}

function sortAssets(list) {
  return [...list].sort((a, b) => {
    const an = (a.displayName || a.hostname || '').toLowerCase()
    const bn = (b.displayName || b.hostname || '').toLowerCase()
    if (an !== bn) return an.localeCompare(bn, localeTag())
    return String(a.id || '').localeCompare(String(b.id || ''))
  })
}

async function reloadAll() {
  await groupTreeRef.value?.reload?.()
  syncSelectedGroupName()
  await loadAssets()
}

async function openInstall() {
  if (selectedKey.value === ALL_KEY) {
    ElMessage.warning(t('assets.selectGroupFirst'))
    return
  }
  const node = groupTreeRef.value?.findGroup?.(selectedKey.value)
  if (!node) {
    ElMessage.warning(t('assets.selectGroupFirst'))
    return
  }
  installLoading.value = true
  try {
    const { data } = await api.post('/install-codes', { groupId: selectedKey.value })
    installGroupName.value = node.name || node.label || ''
    installCode.value = data.code || ''
    installCurl.value = data.curl || ''
    installPowershell.value = data.powershell || ''
    installCmd.value = data.cmd || ''
    const exp = data.expiresAt ? Date.parse(data.expiresAt) : NaN
    installExpiresAt.value = Number.isFinite(exp) ? exp : Date.now() + 15 * 60 * 1000
    curlTipOpen.value = false
    proxyTipOpen.value = false
    startInstallCountdown()
    installVisible.value = true
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('assets.generateFailed'))
  } finally {
    installLoading.value = false
  }
}

async function copyText(text) {
  await navigator.clipboard.writeText(text)
  ElMessage.success(t('common.copied'))
}

onMounted(async () => {
  await reloadAll()
  assetsPollTimer = setInterval(loadAssets, 15000)
})
onBeforeUnmount(() => {
  stopInstallCountdown()
  if (assetsPollTimer != null) {
    clearInterval(assetsPollTimer)
    assetsPollTimer = null
  }
})
</script>

<style scoped>
.assets-page { display: flex; flex-direction: column; height: calc(100vh - 100px); min-height: 480px; }
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  flex-shrink: 0;
}
.toolbar-search { flex: 1; max-width: 360px; min-width: 180px; }
.toolbar-actions { display: flex; gap: 8px; flex-shrink: 0; margin-left: auto; }
.toolbar-actions :deep(.el-button),
.list-pane :deep(.el-button),
.install-block :deep(.el-button) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.explorer { display: flex; gap: 12px; flex: 1; min-height: 0; }
.list-pane {
  flex: 1; min-width: 0; display: flex; flex-direction: column;
  border: 1px solid var(--ops-border); border-radius: var(--ops-radius);
  padding: 8px; overflow: hidden; background: var(--ops-bg-surface);
}
.list-title {
  display: flex; justify-content: space-between; align-items: center;
  font-weight: 600; margin-bottom: 8px; font-size: 13px; flex-shrink: 0; gap: 12px;
  color: var(--ops-text);
}
.list-title-right {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
  font-weight: 500;
}
.list-count {
  color: var(--ops-text-secondary);
  font-size: 12px;
  white-space: nowrap;
}
.os-cell { display: flex; align-items: center; min-width: 0; gap: 6px; }
.os-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.proto { flex-shrink: 0; }
.row-actions {
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
}
.row-actions :deep(.el-button) {
  padding: 5px 8px;
}
.install-dialog-title { font-size: 18px; font-weight: 600; line-height: 1.4; color: var(--ops-text); }
.install-countdown { color: var(--ops-danger); font-weight: 700; }
.install-countdown.expired { color: var(--ops-danger); }
.install-target { color: var(--ops-text); font-size: 16px; line-height: 1.5; margin: 0 0 16px; }
.install-group { color: var(--ops-danger); font-weight: 700; }
.install-ttl { display: block; margin-top: 6px; color: var(--ops-text-secondary); font-size: 13px; font-weight: 500; }
.install-ttl.expired { color: var(--ops-danger); font-weight: 700; }
.install-block { margin-bottom: 16px; }
.install-label { font-weight: 600; margin-bottom: 6px; }
.install-code-hint { margin: 0 0 8px; font-size: 13px; color: var(--ops-text-secondary); line-height: 1.5; }
.copy-btn { margin-top: 8px; }
.install-curl-tip {
  margin-top: 10px;
  padding: 8px 12px;
  background: var(--ops-bg-page);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  font-size: 13px;
  color: var(--ops-text);
  line-height: 1.5;
}
.install-curl-tip-toggle {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  line-height: 1.5;
}
.install-curl-tip-toggle:hover { color: var(--ops-text); }
.install-curl-tip-chevron {
  flex-shrink: 0;
  margin-top: 2px;
  transition: transform 0.15s ease;
}
.install-curl-tip-chevron.open { transform: rotate(180deg); }
.install-curl-tip-body { margin-top: 8px; }
.install-curl-tip-body p { margin: 0 0 8px; }
.install-curl-tip code {
  font-size: 12px;
  font-family: var(--ops-font-mono);
  background: var(--ops-border);
  padding: 1px 4px;
  border-radius: 3px;
}
.install-proxy-note {
  margin-top: 10px;
  padding: 8px 12px;
  background: var(--ops-bg-page);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  color: var(--ops-text);
  font-size: 13px;
  line-height: 1.55;
}
.install-proxy-note ul { margin: 0 0 8px; padding-left: 1.25em; }
.install-proxy-note li { margin-bottom: 4px; }
.install-proxy-note code {
  font-size: 12px;
  font-family: var(--ops-font-mono);
  padding: 1px 5px;
  background: var(--ops-color-primary-soft);
  border-radius: 3px;
}
.install-proxy-examples { font-weight: 600; margin: 8px 0 4px !important; }
.install-proxy-pre {
  margin: 0 0 8px;
  padding: 8px 10px;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.45;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.install-proxy-foot { margin: 0 !important; color: #64748b; font-size: 12px; }
</style>
