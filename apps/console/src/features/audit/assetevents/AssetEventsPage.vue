<template>
  <div class="page">
    <div class="page-toolbar">
      <div class="filters">
        <el-select
          v-model="filters.category"
          clearable
          :placeholder="t('audit.assetEvents.categoryPlaceholder')"
          style="width:140px"
          @change="onFilterChange"
        >
          <el-option :label="t('audit.assetEvents.catConnectivity')" value="CONNECTIVITY" />
          <el-option :label="t('audit.assetEvents.catNetwork')" value="NETWORK" />
        </el-select>
        <el-select
          v-model="filters.eventType"
          clearable
          :placeholder="t('audit.assetEvents.eventPlaceholder')"
          style="width:150px"
          @change="onFilterChange"
        >
          <el-option :label="t('audit.assetEvents.online')" value="ONLINE" />
          <el-option :label="t('audit.assetEvents.offline')" value="OFFLINE" />
          <el-option :label="t('audit.assetEvents.privateIpChanged')" value="PRIVATE_IP_CHANGED" />
        </el-select>
        <AssetTreeSelect
          v-model="filters.assetId"
          :placeholder="t('audit.assetEvents.assetPlaceholder')"
          width="260px"
          @change="onFilterChange"
        />
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-table :data="rows" v-loading="loading" stripe size="small" :empty-text="t('audit.assetEvents.empty')">
      <el-table-column :label="t('common.time')" width="150">
        <template #default="{ row }">{{ formatTime(row.occurredAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.assetEvents.colCategory')" width="88">
        <template #default="{ row }">{{ categoryLabel(row.category) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.assetEvents.colEvent')" width="110">
        <template #default="{ row }">
          <el-tag size="small" :type="eventTagType(row.eventType)">
            {{ eventTypeLabel(row.eventType) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.asset')" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.assetDisplayName || row.hostname || row.assetId || t('common.emDash') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.group')" width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.groupName || t('common.ungrouped') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.detailCol')" min-width="260">
        <template #default="{ row }">
          <el-tooltip
            placement="top"
            :show-after="300"
            :disabled="!detailTip(row.detail)"
          >
            <template #content>
              <div class="tip">{{ detailTip(row.detail) }}</div>
            </template>
            <span class="detail-cell">{{ detailSummary(row.detail) || t('common.emDash') }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
    <div class="pager">
      <el-pagination
        v-model:current-page="pager.page"
        v-model:page-size="pager.pageSize"
        :total="pager.total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        background
        small
        @current-change="load"
        @size-change="onPageSizeChange"
      />
    </div>
    <p class="hint">{{ t('audit.assetEvents.hint') }}</p>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import api from '../../../shared/api'
import AssetTreeSelect from '../../../shared/AssetTreeSelect.vue'
import { formatTime } from '../../../shared/format'

const { t } = useI18n()
const route = useRoute()
const loading = ref(false)
const rows = ref([])
const filters = reactive({ category: '', eventType: '', assetId: '' })
const pager = reactive({ page: 1, pageSize: 50, total: 0 })

function onFilterChange() {
  pager.page = 1
  load()
}

function onPageSizeChange() {
  pager.page = 1
  load()
}

function categoryLabel(c) {
  return ({
    CONNECTIVITY: t('audit.assetEvents.catConnectivity'),
    NETWORK: t('audit.assetEvents.catNetwork')
  })[c] || c
}

function eventTypeLabel(type) {
  return ({
    ONLINE: t('audit.assetEvents.online'),
    OFFLINE: t('audit.assetEvents.offline'),
    PRIVATE_IP_CHANGED: t('audit.assetEvents.privateIpChanged')
  })[type] || type
}

function eventTagType(type) {
  if (type === 'ONLINE') return 'success'
  if (type === 'OFFLINE') return 'info'
  if (type === 'PRIVATE_IP_CHANGED') return 'warning'
  return ''
}

function parseDetail(raw) {
  if (!raw || raw === '{}') return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function reasonLabel(r) {
  return ({
    connected: t('audit.assetEvents.reasonConnected'),
    read_timeout: t('audit.assetEvents.reasonReadTimeout'),
    client_close: t('audit.assetEvents.reasonClientClose'),
    abnormal_close: t('audit.assetEvents.reasonAbnormalClose'),
    gateway_close: t('audit.assetEvents.reasonGatewayClose'),
    disconnected: t('audit.assetEvents.reasonDisconnected')
  })[r] || r
}

/** List cell: IPs / version / reason — no gw/conn noise. */
function detailSummary(raw) {
  const o = parseDetail(raw)
  if (!o) return typeof raw === 'string' && raw && raw !== '{}' ? raw : ''
  const parts = []
  if (o.from != null || o.to != null) {
    parts.push(`${o.from || t('common.emDash')} → ${o.to || t('common.emDash')}`)
  }
  if (o.sourceIp) parts.push(t('audit.assetEvents.publicIp', { ip: o.sourceIp }))
  if (o.privateIp) parts.push(t('audit.assetEvents.privateIp', { ip: o.privateIp }))
  if (o.agentVersion) parts.push(`ver:${o.agentVersion}`)
  if (o.reason) parts.push(reasonLabel(o.reason))
  return parts.join(' · ')
}

/** Hover tip: summary + gateway / connection ids. */
function detailTip(raw) {
  const o = parseDetail(raw)
  if (!o) return typeof raw === 'string' && raw && raw !== '{}' ? raw : ''
  const parts = []
  const summary = detailSummary(raw)
  if (summary) parts.push(summary)
  if (o.gatewayInstance) parts.push(`Gateway:${o.gatewayInstance}`)
  if (o.connectionId) parts.push(t('audit.assetEvents.connection', { id: o.connectionId }))
  if (o.peerIp && o.peerIp !== o.sourceIp) parts.push(`peer:${o.peerIp}`)
  return parts.join('\n')
}

async function load() {
  loading.value = true
  try {
    const params = {
      page: pager.page,
      pageSize: pager.pageSize
    }
    if (filters.category) params.category = filters.category
    if (filters.eventType) params.eventType = filters.eventType
    if (filters.assetId) params.assetId = filters.assetId
    const { data } = await api.get('/asset-events', { params })
    rows.value = data?.items || []
    pager.total = data?.total ?? 0
    if (data?.page) pager.page = data.page
    if (data?.pageSize) pager.pageSize = data.pageSize
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  const q = route.query.assetId
  if (typeof q === 'string' && q) filters.assetId = q
  await load()
})
</script>

<style scoped>
.page-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.filters { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.detail-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
  cursor: default;
}
.tip { white-space: pre-line; max-width: 420px; line-height: 1.5; }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.hint { margin-top: 4px; }
</style>
