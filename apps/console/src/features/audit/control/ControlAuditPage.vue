<template>
  <div class="page">
    <div class="page-toolbar">
      <div class="filters">
        <el-select
          v-model="filters.category"
          clearable
          :placeholder="t('audit.control.categoryPlaceholder')"
          style="width:130px"
          @change="onFilterChange"
        >
          <el-option :label="t('audit.control.catAuth')" value="AUTH" />
          <el-option :label="t('audit.control.catUser')" value="USER" />
          <el-option :label="t('audit.control.catAsset')" value="ASSET" />
          <el-option :label="t('audit.control.catGroup')" value="GROUP" />
          <el-option :label="t('audit.control.catPortmap')" value="PORTMAP" />
          <el-option :label="t('audit.control.catCi')" value="CI" />
          <el-option :label="t('audit.control.catMonitor')" value="MONITOR" />
        </el-select>
        <AssetTreeSelect
          v-model="filters.assetId"
          :placeholder="t('audit.control.assetPlaceholder')"
          width="260px"
          @change="onFilterChange"
        />
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <el-table :data="rows" v-loading="loading" stripe size="small">
      <el-table-column :label="t('common.time')" width="170">
        <template #default="{ row }">{{ formatTime(row.occurredAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.control.colCategory')" width="100">
        <template #default="{ row }">{{ categoryLabel(row.category) }}</template>
      </el-table-column>
      <el-table-column :label="t('audit.control.colAction')" width="110">
        <template #default="{ row }">{{ actionLabel(row.action) }}</template>
      </el-table-column>
      <el-table-column prop="username" :label="t('common.user')" width="120">
        <template #default="{ row }">{{ row.username || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.asset')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.assetDisplayName || row.hostname || row.assetId || t('common.emDash') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.group')" width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.groupName || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.detailCol')" min-width="240" show-overflow-tooltip>
        <template #default="{ row }">{{ formatDetail(row.detail) }}</template>
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
    <p class="hint">{{ t('audit.control.hint') }}</p>
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
const filters = reactive({ category: '', assetId: '' })
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
    AUTH: t('audit.control.catAuth'),
    USER: t('audit.control.catUser'),
    ASSET: t('audit.control.catAsset'),
    GROUP: t('audit.control.catGroup'),
    PORTMAP: t('audit.control.catPortmap'),
    CI: t('audit.control.catCi'),
    MONITOR: t('audit.control.catMonitor')
  })[c] || c
}

function actionLabel(a) {
  return ({
    LOGIN: t('audit.control.actionLogin'),
    CREATE: t('audit.control.actionCreate'),
    UPDATE: t('audit.control.actionUpdate'),
    REMOVE: t('audit.control.actionRemove'),
    SET_SCOPES: t('audit.control.actionSetScopes'),
    REGISTER: t('audit.control.actionRegister'),
    REREGISTER: t('audit.control.actionReregister'),
    INSTALL_CODE: t('audit.control.actionInstallCode'),
    REVOKE_INSTALL: t('audit.control.actionRevokeInstall'),
    REVOKE: t('audit.control.actionRevoke'),
    UPDATE_AGENT: t('audit.control.actionUpdateAgent'),
    TOTP_ENABLE: t('audit.control.actionTotpEnable'),
    TOTP_RESET: t('audit.control.actionTotpReset'),
    IGNORE: t('audit.control.actionIgnore'),
    UNIGNORE: t('audit.control.actionUnignore')
  })[a] || a
}

function formatDetail(raw) {
  if (!raw) return ''
  try {
    const o = JSON.parse(raw)
    const parts = []
    if (o.targetUsername) parts.push(t('audit.control.detailUser', { name: o.targetUsername }))
    if (o.role) parts.push(t('audit.control.detailRole', { role: o.role }))
    if (o.enabled != null) {
      parts.push(o.enabled ? t('audit.control.detailEnabled') : t('audit.control.detailDisabled'))
    }
    if (o.passwordChanged) parts.push(t('audit.control.detailPasswordChanged'))
    if (o.groupIds) {
      parts.push(t('audit.control.detailGroups', {
        n: Array.isArray(o.groupIds) ? o.groupIds.length : o.groupIds
      }))
    }
    if (o.name) parts.push(o.name)
    if (o.displayName) parts.push(o.displayName)
    if (o.hostname) parts.push(o.hostname)
    if (o.authSource) parts.push(o.authSource)
    if (o.protocol) parts.push(o.protocol)
    if (o.shellKind) parts.push(o.shellKind)
    if (o.listenPort != null) parts.push(`listen:${o.listenPort}`)
    if (o.targetHost) parts.push(`${o.targetHost}${o.targetPort != null ? ':' + o.targetPort : ''}`)
    if (o.clientAddr) parts.push(`client:${o.clientAddr}`)
    if (o.bytesIn != null || o.bytesOut != null) {
      parts.push(`↓${o.bytesIn ?? 0} ↑${o.bytesOut ?? 0}`)
    }
    if (o.itemId) parts.push(o.itemId)
    return parts.length ? parts.join(' · ') : raw
  } catch {
    return raw
  }
}

async function load() {
  loading.value = true
  try {
    const params = {
      page: pager.page,
      pageSize: pager.pageSize
    }
    if (filters.category) params.category = filters.category
    if (filters.assetId) params.assetId = filters.assetId
    const { data } = await api.get('/control-audit', { params })
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
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.hint { margin-top: 4px; }
</style>
