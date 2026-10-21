<template>
  <div class="home" v-loading="loading">
    <NetworkTopology
      ref="topologyRef"
      :topology="topology"
      :groups="groups"
      :abnormal-assets="summary.abnormalAssets || []"
      :loading="loading"
      :insets="{ top: 130, right: 410, bottom: 56, left: 24 }"
      @open-asset="goAsset"
      @open-monitor="openMonitor"
    />

    <div class="kpi-strip">
      <button type="button" class="stat" @click="goAssets">
        <span class="stat-icon total"><IconServer :size="18" stroke="1.75" /></span>
        <span>
          <span class="label">{{ t('home.totalAssets') }}</span>
          <strong class="value">{{ summary.assetTotal ?? t('common.emDash') }}</strong>
        </span>
      </button>
      <button type="button" class="stat" @click="goAssets">
        <span class="stat-icon online"><IconCircleCheck :size="18" stroke="1.75" /></span>
        <span>
          <span class="label">{{ t('home.online') }}</span>
          <strong class="value ok">{{ summary.assetOnline ?? t('common.emDash') }}</strong>
        </span>
      </button>
      <button type="button" class="stat" @click="focusFirstAbnormal">
        <span class="stat-icon alert"><IconAlertTriangle :size="18" stroke="1.75" /></span>
        <span>
          <span class="label">{{ t('home.abnormal') }}</span>
          <strong class="value" :class="{ bad: (summary.assetAbnormal || 0) > 0 }">
            {{ summary.assetAbnormal ?? t('common.emDash') }}
          </strong>
        </span>
      </button>
      <el-tooltip :content="t('common.refresh')" placement="bottom">
        <button type="button" class="refresh-btn" :disabled="loading" @click="load(true)">
          <IconRefresh :size="17" stroke="1.75" />
        </button>
      </el-tooltip>
    </div>

    <aside class="incidents-panel">
      <div class="incidents-head">
        <div>
          <span class="eyebrow">{{ t('home.liveStatus') }}</span>
          <h3>{{ t('home.abnormalAssets') }}</h3>
        </div>
        <el-button size="small" @click="ignoreVisible = true">
          {{ t('home.ignoredAlerts') }}
          <template v-if="ignoredCount"> ({{ ignoredCount }})</template>
        </el-button>
      </div>
      <el-table
        :data="summary.abnormalAssets || []"
        size="small"
        height="100%"
        :empty-text="t('home.emptyAbnormal')"
        class="incidents-table"
        @row-click="focusAbnormal"
      >
        <el-table-column :label="t('common.name')" min-width="132" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="stack-cell">
              <div class="stack-primary">
                <i class="health-dot" :class="row.online ? 'warning' : 'danger'" />
                {{ row.displayName || row.hostname || t('common.emDash') }}
              </div>
              <div class="stack-secondary">
                {{ row.groupName || t('common.ungrouped') }} · {{ row.privateIp || t('common.emDash') }}
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('home.alertSummary')" min-width="142">
          <template #default="{ row }">
            <div class="issue-list" @click.stop>
              <el-tag
                v-for="issue in issuesOf(row).slice(0, 2)"
                :key="issue.itemId + ':' + (issue.instance || '')"
                size="small"
                :type="issue.kind === 'offline' ? 'info' : 'danger'"
                closable
                :disable-transitions="true"
                @close="ignoreIssue(row, issue)"
              >{{ issueLabel(issue) }}</el-tag>
              <span v-if="issuesOf(row).length > 2" class="more-issues">+{{ issuesOf(row).length - 2 }}</span>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </aside>

    <IgnoredAlertsDialog
      v-model="ignoreVisible"
      :rows="summary.ignoredAlerts || []"
      @changed="load(false)"
    />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  IconAlertTriangle,
  IconCircleCheck,
  IconRefresh,
  IconServer
} from '@tabler/icons-vue'
import api from '../../shared/api'
import IgnoredAlertsDialog from './IgnoredAlertsDialog.vue'
import NetworkTopology from './NetworkTopology.vue'
import { formatAlertDuration } from './alertDuration'

const { t } = useI18n()
const router = useRouter()

const loading = ref(false)
const summary = ref({})
const topology = ref({ nodes: [], edges: [] })
const groups = ref([])
const ignoreVisible = ref(false)
const topologyRef = ref(null)
const ignoredCount = computed(() => (summary.value.ignoredAlerts || []).length)
let refreshTimer = null

function goAsset(row) {
  const q = String(row.name || row.displayName || row.hostname || '').trim()
  router.push({ path: '/assets', query: q ? { q } : {} })
}

function goAssets() {
  router.push('/assets')
}

function focusAbnormal(row) {
  topologyRef.value?.focusAsset(row.assetId)
}

function focusFirstAbnormal() {
  const first = summary.value.abnormalAssets?.[0]
  if (first?.assetId) focusAbnormal(first)
}

function issuesOf(row) {
  if (Array.isArray(row.issues) && row.issues.length) return row.issues
  if (row.online === false) {
    return [{ itemId: 'host.online', kind: 'offline', label: '', instance: '' }]
  }
  if (row.summary) {
    return [{ itemId: row.summary, kind: 'metric', label: row.summary, instance: '' }]
  }
  return []
}

function issueLabel(issue) {
  if (!issue) return ''
  const base = (issue.kind === 'offline' || issue.itemId === 'host.online')
    ? t('common.offline')
    : (issue.label || issue.itemId)
  const dur = formatAlertDuration(issue.startedAt)
  return dur ? `${base} · ${dur}` : base
}

function openMonitor(row) {
  const id = row.assetId || row.id
  if (!id) return
  const name = `ops-monitor-${id}-${Date.now()}`
  const win = window.open(`/assets/${id}/monitor`, name)
  if (!win) {
    ElMessage.warning(t('home.popupBlocked'))
  }
}

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const [summaryRes, topologyRes, groupsRes] = await Promise.all([
      api.get('/dashboard/summary'),
      api.get('/assets/update-topology'),
      api.get('/groups')
    ])
    summary.value = summaryRes.data || {}
    topology.value = topologyRes.data || { nodes: [], edges: [] }
    groups.value = groupsRes.data || []
  } finally {
    if (showLoading) loading.value = false
  }
}

async function ignoreIssue(row, issue) {
  try {
    await ElMessageBox.confirm(
      t('home.ignoreConfirm', { name: row.displayName || row.hostname, item: issueLabel(issue) }),
      t('home.ignoreTitle')
    )
  } catch {
    return
  }
  try {
    await api.post(`/assets/${row.assetId}/alert-ignores`, { itemId: issue.itemId })
    ElMessage.success(t('home.ignoreSuccess'))
    await load(false)
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('home.ignoreFailed'))
  }
}

onMounted(async () => {
  await load()
  refreshTimer = setInterval(() => load(false), 60_000)
})

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.home {
  gap: 20px;
}

.cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(140px, 240px));
  gap: 12px;
}

.stat {
  background: var(--ops-bg-surface);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  padding: 16px 18px;
}

.stat .label {
  color: var(--ops-text-secondary);
  font-size: 13px;
  font-weight: 500;
}

.stat .value {
  margin-top: 8px;
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.03em;
  color: var(--ops-text);
  font-variant-numeric: tabular-nums;
}

.stat.ok .value {
  color: var(--ops-success);
}

.stat.bad .value {
  color: var(--ops-danger);
}

.section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.table-wrap {
  padding: 0;
  overflow: hidden;
}

.table-wrap :deep(.el-table) {
  --el-table-header-bg-color: #f0f3f6;
}

.issue-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 4px 0;
}

.link-btn {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0;
  margin: 0;
  color: var(--el-color-primary);
  font: inherit;
  cursor: pointer;
  text-align: left;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.link-btn:hover {
  color: var(--el-color-primary-light-3);
}

.stack-cell {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 0;
  min-width: 0;
  line-height: 1.15;
  padding: 1px 0;
}

.stack-primary {
  font-size: inherit;
  font-weight: inherit;
  max-width: 100%;
}

.stack-secondary {
  margin-top: 1px;
  font-size: 10px;
  color: #b6c0cc;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Full-screen NOC canvas. Rules below intentionally override the legacy
   card/table styles above while keeping ignored-alert dialog classes intact. */
.home {
  position: relative;
  height: calc(100vh - var(--ops-header-height));
  min-height: 600px;
  margin: -16px -20px;
  overflow: hidden;
  gap: 0;
}
.kpi-strip {
  position: absolute;
  z-index: 5;
  top: 18px;
  left: 18px;
  display: grid;
  grid-template-columns: repeat(3, minmax(132px, 168px)) 36px;
  gap: 8px;
}
.kpi-strip .stat {
  appearance: none;
  min-width: 0;
  height: 66px;
  padding: 10px 13px;
  display: flex;
  align-items: center;
  gap: 11px;
  text-align: left;
  border: 1px solid rgba(197, 206, 217, 0.92);
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 9px 28px rgba(27, 36, 48, 0.1);
  backdrop-filter: blur(12px);
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.kpi-strip .stat:hover { transform: translateY(-1px); box-shadow: 0 12px 32px rgba(27, 36, 48, 0.14); }
.kpi-strip .stat > span:last-child { min-width: 0; }
.stat-icon {
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}
.stat-icon.total { color: #2f5d9f; background: #e8eef6; }
.stat-icon.online { color: #1f7a4d; background: #e7f5ed; }
.stat-icon.alert { color: #b54708; background: #fff2df; }
.kpi-strip .stat .label {
  display: block;
  color: var(--ops-text-secondary);
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
}
.kpi-strip .stat .value {
  display: block;
  margin-top: 1px;
  font-size: 23px;
  font-weight: 600;
  letter-spacing: -0.03em;
  color: var(--ops-text);
  font-variant-numeric: tabular-nums;
}
.kpi-strip .stat .value.ok { color: var(--ops-success); }
.kpi-strip .stat .value.bad { color: var(--ops-danger); }
.refresh-btn {
  appearance: none;
  width: 36px;
  height: 36px;
  align-self: center;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(197, 206, 217, 0.92);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  color: var(--ops-text-secondary);
  box-shadow: 0 7px 22px rgba(27, 36, 48, 0.09);
  cursor: pointer;
}
.refresh-btn:hover { color: var(--ops-color-primary); }
.refresh-btn:disabled { cursor: default; opacity: 0.6; }
.incidents-panel {
  position: absolute;
  z-index: 5;
  top: 18px;
  right: 18px;
  bottom: 18px;
  width: 380px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid rgba(197, 206, 217, 0.92);
  border-radius: 10px;
  background: rgba(255, 255,255, 0.93);
  box-shadow: 0 16px 42px rgba(27, 36, 48, 0.13);
  backdrop-filter: blur(14px);
}
.incidents-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 14px 14px 11px;
  border-bottom: 1px solid var(--ops-border);
}
.incidents-head h3 { margin: 1px 0 0; font-size: 15px; font-weight: 600; color: var(--ops-text); }
.eyebrow {
  color: var(--ops-text-muted);
  font-family: var(--ops-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.incidents-table {
  flex: 1;
  min-height: 0;
  --el-table-bg-color: transparent;
  --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: rgba(240, 243, 246, 0.72);
}
.incidents-table :deep(.el-table__row) { cursor: pointer; }
.incidents-table :deep(.el-table__cell) { padding: 8px 0; }
.issue-list {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 1px 0;
}
.issue-list :deep(.el-tag) { max-width: 100%; }
.issue-list :deep(.el-tag__content) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.more-issues { color: var(--ops-text-muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.incidents-panel .stack-primary {
  display: flex;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.incidents-panel .stack-secondary {
  margin-top: 3px;
  padding-left: 14px;
  color: var(--ops-text-muted);
}
.health-dot { width: 8px; height: 8px; flex: 0 0 8px; border-radius: 50%; }
.health-dot.warning { background: #d97706; box-shadow: 0 0 0 3px rgba(217, 119, 6, 0.12); }
.health-dot.danger { background: #dc2626; box-shadow: 0 0 0 3px rgba(220, 38, 38, 0.12); }
@media (max-width: 1180px) {
  .incidents-panel { width: 330px; }
  .kpi-strip { grid-template-columns: repeat(3, minmax(116px, 144px)) 36px; }
}
@media (max-width: 900px) {
  .home {
    height: auto;
    min-height: calc(100vh - var(--ops-header-height));
    overflow: auto;
    background: #f7f9fc;
  }
  .home :deep(.network-topology) { height: 600px; }
  .kpi-strip {
    right: 12px;
    left: 12px;
    grid-template-columns: repeat(3, minmax(0, 1fr)) 36px;
  }
  .stat-icon { display: none; }
  .incidents-panel {
    position: relative;
    inset: auto;
    width: auto;
    height: 420px;
    margin: 12px;
  }
}
</style>
