<template>
  <div class="page">
    <div class="page-toolbar">
      <div class="filters">
        <el-select
          v-model="filters.operationType"
          clearable
          :placeholder="t('audit.operations.typePlaceholder')"
          style="width:150px"
          @change="onFilterChange"
        >
          <el-option v-for="opt in typeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
        <AssetTreeSelect
          v-model="filters.assetId"
          :placeholder="t('audit.operations.assetPlaceholder')"
          width="260px"
          @change="onFilterChange"
        />
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="tabs" @tab-change="onTabChange">
      <el-tab-pane :label="t('audit.operations.tabSession')" name="session" />
      <el-tab-pane :label="t('audit.operations.tabPortmap')" name="portmap" />
    </el-tabs>

    <el-table :data="rows" v-loading="loading" stripe size="small" :empty-text="t('audit.operations.empty')">
      <el-table-column :label="t('common.time')" width="170">
        <template #default="{ row }">{{ formatDateTime(row.occurredAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.type')" width="110">
        <template #default="{ row }">{{ operationTypeLabel(row.operationType) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.asset')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.assetDisplayName || row.hostname || row.assetId || t('common.emDash') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.group')" width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.groupName || t('common.ungrouped') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.user')" width="120">
        <template #default="{ row }">{{ row.username || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.status')" width="96" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.duration')" width="90" align="center">
        <template #default="{ row }">{{ formatDuration(row.occurredAt, row.endedAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.summary')" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ formatDetail(row.detail) || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="150" fixed="right" align="center">
        <template #default="{ row }">
          <el-button size="small" link type="primary" @click="openDetail(row)">{{ t('common.detail') }}</el-button>
          <el-button
            v-if="recordingPath(row)"
            size="small"
            link
            type="primary"
            @click="openPlay(row)"
          >
            {{ t('audit.operations.replay') }}
          </el-button>
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
    <p class="hint">
      {{ activeTab === 'session' ? t('audit.operations.hintSession') : t('audit.operations.hintPortmap') }}
    </p>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import api from '../../../shared/api'
import AssetTreeSelect from '../../../shared/AssetTreeSelect.vue'
import {
  PORTMAP_OPERATION_TYPES,
  SESSION_OPERATION_TYPES,
  formatDateTime,
  formatDetail,
  formatDuration,
  operationTypeLabel,
  recordingPath,
  statusLabel,
  statusTagType
} from '../shared/operationFormat'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const rows = ref([])
const activeTab = ref('session')
const filters = reactive({ assetId: '', operationType: '' })
const pager = reactive({ page: 1, pageSize: 50, total: 0 })

const typeOptions = computed(() => (
  activeTab.value === 'portmap' ? PORTMAP_OPERATION_TYPES : SESSION_OPERATION_TYPES
))

function onFilterChange() {
  pager.page = 1
  load()
}

function onTabChange() {
  filters.operationType = ''
  pager.page = 1
  load()
}

function onPageSizeChange() {
  pager.page = 1
  load()
}

function openDetail(row) {
  router.push(`/audit/operations/${row.operationId}`)
}

function openPlay(row) {
  router.push(`/audit/operations/${row.operationId}/play`)
}

async function load() {
  loading.value = true
  try {
    const params = {
      page: pager.page,
      pageSize: pager.pageSize,
      scope: activeTab.value === 'portmap' ? 'portmap' : 'session'
    }
    if (filters.assetId) params.assetId = filters.assetId
    if (filters.operationType) params.operationType = filters.operationType
    const { data } = await api.get('/server-operations', { params })
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
  const tab = route.query.tab
  if (tab === 'portmap' || tab === 'session') activeTab.value = tab
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
.tabs { margin-bottom: 8px; }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.hint { margin-top: 4px; }
</style>
