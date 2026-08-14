<template>
  <div class="page" v-loading="loading">
    <div class="page-toolbar">
      <el-button link @click="back">{{ t('audit.play.back') }}</el-button>
      <div class="actions">
        <el-button v-if="recordingPath(operation)" type="primary" size="small" @click="play">
          {{ t('audit.detail.replay', { suffix: formatSuffix }) }}
        </el-button>
        <el-button size="small" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />

    <el-descriptions v-if="operation" :column="2" border size="small" class="summary">
      <el-descriptions-item :label="t('common.asset')">
        {{ operation.assetDisplayName || operation.hostname || operation.assetId }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('common.group')">
        {{ operation.groupName || t('common.ungrouped') }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('common.user')">
        {{ operation.username || t('common.emDash') }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('common.status')">
        <el-tag size="small" :type="statusTagType(operation.status)">
          {{ statusLabel(operation.status) }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item :label="t('audit.detail.start')">
        {{ formatDateTime(operation.occurredAt) }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('audit.detail.end')">
        {{ operation.endedAt ? formatDateTime(operation.endedAt) : t('common.running') }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('common.duration')">
        {{ formatDuration(operation.occurredAt, operation.endedAt) }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('audit.detail.operationId')">
        {{ operation.operationId }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('audit.detail.recording')" :span="2">
        <template v-if="recordingPath(operation)">
          {{ recordingFormatLabel(operation.detail?.format) }} ·
          {{ formatBytes(operation.detail?.size) }} ·
          <code class="mono">{{ recordingPath(operation) }}</code>
        </template>
        <template v-else>{{ t('audit.detail.noRecording') }}</template>
      </el-descriptions-item>
      <el-descriptions-item :label="t('common.detailCol')" :span="2">
        <pre class="detail-json">{{ prettyDetail }}</pre>
      </el-descriptions-item>
    </el-descriptions>

    <h3 class="events-title">{{ t('audit.detail.eventsTitle', { n: events.length }) }}</h3>
    <el-table :data="events" stripe size="small" :empty-text="t('audit.detail.emptyEvents')">
      <el-table-column :label="t('common.time')" width="180">
        <template #default="{ row }">{{ formatDateTime(row.occurredAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.detail.colEvent')" width="130">
        <template #default="{ row }">{{ eventTypeLabel(row.eventType) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.detail.colPathCmd')" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.detail?.command" class="mono">{{ row.detail.command }}</span>
          <template v-else>
            <span class="mono">{{ row.detail?.path || t('common.emDash') }}</span>
            <span v-if="row.detail?.to" class="mono"> → {{ row.detail.to }}</span>
          </template>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.size')" width="110" align="right">
        <template #default="{ row }">
          {{ row.detail?.bytes != null ? formatBytes(row.detail.bytes) : t('common.emDash') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('audit.detail.colOther')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ formatDetail(row.detail) || t('common.emDash') }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import api from '../../../shared/api'
import {
  eventTypeLabel,
  formatBytes,
  formatDateTime,
  formatDetail,
  formatDuration,
  operationTypeLabel,
  recordingFormatLabel,
  recordingPath,
  statusLabel,
  statusTagType
} from '../shared/operationFormat'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const error = ref('')
const operation = ref(null)
const events = ref([])

const prettyDetail = computed(() => {
  const d = operation.value?.detail
  return d ? JSON.stringify(d, null, 2) : t('common.emDash')
})

const formatSuffix = computed(() => {
  const label = recordingFormatLabel(operation.value?.detail?.format)
  return label ? `（${label}）` : ''
})

function back() {
  router.push('/audit/operations')
}

function play() {
  router.push(`/audit/operations/${route.params.id}/play`)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get(`/server-operations/${route.params.id}`)
    operation.value = data?.operation || null
    events.value = data?.events || []
  } catch (e) {
    error.value = e?.response?.data?.error || e?.message || t('common.loadFailed')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.page-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}
.actions { display: flex; align-items: center; gap: 8px; }
.summary { margin-bottom: 16px; }
.events-title { margin: 0 0 8px; font-size: 15px; }
.mono,
.detail-json {
  font-family: var(--ops-font-mono);
  font-size: 12px;
}
.detail-json { margin: 0; max-height: 180px; overflow: auto; }
</style>
