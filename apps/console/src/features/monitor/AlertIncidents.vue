<template>
  <div class="page" v-loading="loading">
    <div class="page-toolbar">
      <AssetTreeSelect v-model="assetId" width="260px" :placeholder="t('monitor.filterAsset')" />
      <el-select v-model="status" style="width:140px" @change="reload">
        <el-option :label="t('monitor.statusAll')" value="" />
        <el-option :label="t('monitor.statusOpen')" value="open" />
        <el-option :label="t('monitor.statusResolved')" value="resolved" />
      </el-select>
      <el-button size="small" @click="reload">{{ t('common.refresh') }}</el-button>
    </div>
    <el-table :data="items" size="small" stripe>
      <el-table-column :label="t('monitor.asset')" min-width="160">
        <template #default="{ row }">
          <div class="name-cell">
            <div>{{ row.assetName }}</div>
            <div class="sub">{{ row.hostname }}</div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="groupName" :label="t('common.group')" min-width="120">
        <template #default="{ row }">{{ row.groupName || t('common.ungrouped') }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.itemId')" min-width="150">
        <template #default="{ row }">
          {{ monitorItemLabel(row.itemId, row.itemId) }}
          <span v-if="row.instance" class="sub"> ({{ row.instance }})</span>
        </template>
      </el-table-column>
      <el-table-column prop="label" :label="t('monitor.summary')" min-width="160" show-overflow-tooltip />
      <el-table-column :label="t('monitor.startedAt')" width="170">
        <template #default="{ row }">{{ fmt(row.startedAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.resolvedAt')" width="170">
        <template #default="{ row }">{{ row.resolvedAt ? fmt(row.resolvedAt) : t('monitor.statusOpen') }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.resolutionReason')" width="130">
        <template #default="{ row }">{{ reasonLabel(row.resolutionReason) }}</template>
      </el-table-column>
    </el-table>
    <div class="pager">
      <el-pagination
        layout="total, prev, pager, next"
        :total="total"
        :page-size="pageSize"
        :current-page="page"
        @current-change="onPage"
      />
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '../../shared/api'
import AssetTreeSelect from '../../shared/AssetTreeSelect.vue'
import { monitorItemLabel } from './monitorItemLabel'

const { t } = useI18n()
const loading = ref(false)
const items = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const assetId = ref('')
const status = ref('')

function fmt(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

function reasonLabel(r) {
  if (!r) return ''
  const key = `monitor.reason.${r}`
  const label = t(key)
  return label !== key ? label : r
}

async function load() {
  loading.value = true
  try {
    const { data } = await api.get('/monitor/alert-incidents', {
      params: {
        page: page.value,
        pageSize,
        status: status.value || undefined,
        assetId: assetId.value || undefined
      }
    })
    items.value = data?.items || []
    total.value = data?.total || 0
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  load()
}

function onPage(p) {
  page.value = p
  load()
}

watch(assetId, reload)
onMounted(load)
</script>

<style scoped>
.page-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.name-cell .sub,
.sub {
  color: var(--ops-text-muted, #888);
  font-size: 12px;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
