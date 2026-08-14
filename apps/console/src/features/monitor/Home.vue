<template>
  <div class="page home" v-loading="loading">
    <div class="cards">
      <div class="stat">
        <div class="label">{{ t('home.totalAssets') }}</div>
        <div class="value">{{ summary.assetTotal ?? t('common.emDash') }}</div>
      </div>
      <div class="stat ok">
        <div class="label">{{ t('home.online') }}</div>
        <div class="value">{{ summary.assetOnline ?? t('common.emDash') }}</div>
      </div>
      <div class="stat" :class="{ bad: (summary.assetAbnormal || 0) > 0 }">
        <div class="label">{{ t('home.abnormal') }}</div>
        <div class="value">{{ summary.assetAbnormal ?? t('common.emDash') }}</div>
      </div>
    </div>

    <div class="section">
      <div class="section-head">
        <h3 class="page-subtitle">{{ t('home.abnormalAssets') }}</h3>
        <el-button size="small" @click="ignoreVisible = true">
          {{ t('home.ignoredAlerts') }}
          <template v-if="ignoredCount"> ({{ ignoredCount }})</template>
        </el-button>
      </div>
      <div class="panel table-wrap">
        <el-table :data="summary.abnormalAssets || []" size="small" stripe :empty-text="t('home.emptyAbnormal')">
          <el-table-column prop="displayName" :label="t('common.name')" min-width="140" />
          <el-table-column prop="hostname" :label="t('home.hostname')" min-width="120" />
          <el-table-column :label="t('common.status')" width="90">
            <template #default="{ row }">
              <el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? t('common.online') : t('common.offline') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('home.alertSummary')" min-width="260">
            <template #default="{ row }">
              <div class="issue-tags">
                <el-tag
                  v-for="issue in issuesOf(row)"
                  :key="issue.itemId + ':' + (issue.instance || '')"
                  size="small"
                  :type="issue.kind === 'offline' ? 'info' : 'danger'"
                  closable
                  :disable-transitions="true"
                  @close="ignoreIssue(row, issue)"
                >{{ issueLabel(issue) }}</el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="100" align="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openMonitor(row)">{{ t('home.openMonitor') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <IgnoredAlertsDialog
      v-model="ignoreVisible"
      :rows="summary.ignoredAlerts || []"
      @changed="load(false)"
    />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import IgnoredAlertsDialog from './IgnoredAlertsDialog.vue'

const { t } = useI18n()

const loading = ref(false)
const summary = ref({})
const ignoreVisible = ref(false)
const ignoredCount = computed(() => (summary.value.ignoredAlerts || []).length)

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
  if (issue.kind === 'offline' || issue.itemId === 'host.online') return t('common.offline')
  return issue.label || issue.itemId
}

function openMonitor(row) {
  const id = row.assetId
  const name = `ops-monitor-${id}-${Date.now()}`
  const win = window.open(`/assets/${id}/monitor`, name)
  if (!win) {
    ElMessage.warning(t('home.popupBlocked'))
  }
}

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const { data } = await api.get('/dashboard/summary')
    summary.value = data || {}
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

onMounted(load)
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
</style>
