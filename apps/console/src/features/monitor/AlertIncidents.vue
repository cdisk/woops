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
    <el-table :data="items" size="small" stripe table-layout="auto">
      <el-table-column :label="t('monitor.asset')" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="name-cell">
            <div class="ellip">{{ row.assetName }}</div>
            <div class="sub ellip">{{ row.hostname }}</div>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.group')" min-width="100" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="ellip">{{ row.groupName || t('common.ungrouped') }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('monitor.itemId')" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="ellip">
            {{ monitorItemLabel(row.itemId, row.itemId) }}<template v-if="row.instance"> ({{ row.instance }})</template>
          </span>
        </template>
      </el-table-column>
      <el-table-column prop="label" :label="t('monitor.summary')" min-width="220" show-overflow-tooltip />
      <el-table-column :label="t('monitor.startedAt')" width="150">
        <template #default="{ row }">{{ fmt(row.startedAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.resolvedAt')" width="150">
        <template #default="{ row }">{{ row.resolvedAt ? fmt(row.resolvedAt) : t('monitor.statusOpen') }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.resolutionReason')" width="100" show-overflow-tooltip>
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
import { ElMessage } from 'element-plus'
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
  } catch (e) {
    items.value = []
    total.value = 0
    ElMessage.error(e.response?.data?.error || t('common.loadFailed'))
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
.name-cell {
  min-width: 0;
  line-height: 1.15;
}
.name-cell .sub {
  color: var(--ops-text-muted, #888);
  font-size: 12px;
}
.ellip {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
