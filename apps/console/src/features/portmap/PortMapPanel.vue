<template>
  <div class="portmap-panel">
    <div class="toolbar">
      <el-input
        v-model="searchQuery"
        clearable
        size="small"
        class="toolbar-search"
        :placeholder="t('portmap.searchPlaceholder')"
      />
      <div class="toolbar-actions">
        <el-button type="primary" size="small" @click="openCreate">{{ t('portmap.create') }}</el-button>
        <el-button size="small" :loading="loading" @click="reload">{{ t('common.refresh') }}</el-button>
        <span v-if="assetFilter" class="hint">{{ t('portmap.onlyThisAsset') }}</span>
      </div>
    </div>

    <el-table :data="filteredRows" v-loading="loading" size="small" stripe :empty-text="t('portmap.empty')">
      <el-table-column v-if="!assetFilter" :label="t('common.asset')" min-width="160">
        <template #default="{ row }">
          <div>{{ row.assetDisplayName || row.hostname || row.assetId }}</div>
          <div class="sub">{{ row.hostname }} · {{ row.groupName || t('common.ungrouped') }}</div>
        </template>
      </el-table-column>
      <el-table-column v-if="!assetFilter" :label="t('common.online')" width="72" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="row.online ? 'success' : 'info'">{{ row.online ? t('common.online') : t('common.offline') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('portmap.direction')" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="isReverse(row) ? 'warning' : ''">
            {{ directionLabel(row) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('portmap.protocol')" width="70" prop="protocol" />
      <el-table-column :label="t('portmap.listen')" min-width="150">
        <template #default="{ row }">
          {{ row.listenHost || (isReverse(row) ? '127.0.0.1' : '0.0.0.0') }}:{{ row.listenPort }}
        </template>
      </el-table-column>
      <el-table-column :label="t('portmap.target')" min-width="140">
        <template #default="{ row }">{{ row.targetHost }}:{{ row.targetPort }}</template>
      </el-table-column>
      <el-table-column :label="t('common.remark')" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.remark || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.status')" width="110" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="statusTagType(row)">{{ statusLabel(row) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('portmap.connections')" width="70" align="center" prop="activeConns" />
      <el-table-column :label="t('portmap.traffic')" min-width="140">
        <template #default="{ row }">
          ↓ {{ formatBytes(row.bytesIn) }} / ↑ {{ formatBytes(row.bytesOut) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="220" fixed="right" align="center">
        <template #default="{ row }">
          <el-button size="small" link type="primary" @click="openEditRemark(row)">{{ t('portmap.remarkBtn') }}</el-button>
          <el-button size="small" link type="primary" @click="openHistory(row)">{{ t('portmap.history') }}</el-button>
          <el-button size="small" link type="danger" @click="remove(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <p v-if="filteredRows.some(r => r.lastError)" class="err-hint">{{ t('portmap.lastErrorHint') }}</p>

    <el-dialog v-model="createVisible" :title="t('portmap.createTitle')" width="520px" destroy-on-close>
      <el-form label-width="108px">
        <el-form-item v-if="!fixedAsset" :label="t('common.asset')">
          <AssetTreeSelect
            v-model="form.assetId"
            :placeholder="t('portmap.selectAsset')"
            show-online
            width="100%"
            :assets="assetOptions"
          />
        </el-form-item>
        <el-form-item :label="t('portmap.direction')">
          <el-radio-group v-model="form.direction">
            <el-radio-button value="gateway_to_asset">{{ t('portmap.dirForward') }}</el-radio-button>
            <el-radio-button value="asset_to_gateway">{{ t('portmap.dirReverse') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('portmap.protocol')">
          <el-radio-group v-model="form.protocol">
            <el-radio-button value="tcp">TCP</el-radio-button>
            <el-radio-button value="udp">UDP</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <template v-if="form.direction === 'asset_to_gateway'">
          <el-form-item :label="t('portmap.listenAddr')">
            <el-input v-model="form.listenHost" placeholder="127.0.0.1" />
          </el-form-item>
          <el-form-item :label="t('portmap.listenPort')">
            <el-input-number
              v-model="form.listenPort"
              :min="0"
              :max="65535"
              controls-position="right"
              :placeholder="t('portmap.portAutoPlaceholder')"
            />
            <span class="inline-hint">{{ t('portmap.portAutoHint') }}</span>
          </el-form-item>
        </template>
        <el-form-item :label="t('portmap.targetHost')">
          <el-input
            v-model="form.targetHost"
            :placeholder="form.direction === 'asset_to_gateway' ? t('portmap.targetHostPlaceholderGw') : '127.0.0.1'"
          />
        </el-form-item>
        <el-form-item :label="t('portmap.targetPort')">
          <el-input-number v-model="form.targetPort" :min="1" :max="65535" controls-position="right" />
        </el-form-item>
        <el-form-item :label="t('common.remark')">
          <el-input v-model="form.remark" maxlength="256" show-word-limit :placeholder="t('portmap.remarkPlaceholder')" />
        </el-form-item>
        <p class="form-tip">
          <template v-if="form.direction === 'gateway_to_asset'">
            {{ t('portmap.tipForward') }}
          </template>
          <template v-else>
            {{ t('portmap.tipReverse') }}
          </template>
        </p>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="create">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remarkVisible" :title="t('portmap.editRemark')" width="420px" destroy-on-close>
      <el-input
        v-model="remarkForm.text"
        maxlength="256"
        show-word-limit
        type="textarea"
        :rows="3"
        :placeholder="t('portmap.remarkPlaceholder')"
      />
      <template #footer>
        <el-button @click="remarkVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="remarkSaving" @click="saveRemark">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="historyVisible" :title="historyTitle" size="55%">
      <el-table :data="historyRows" v-loading="historyLoading" size="small" :empty-text="t('portmap.emptyHistory')">
        <el-table-column :label="t('portmap.direction')" width="100">
          <template #default="{ row }">{{ directionLabel(row) || t('common.emDash') }}</template>
        </el-table-column>
        <el-table-column :label="t('portmap.listen')" min-width="120">
          <template #default="{ row }">
            <span v-if="row.listenPort">{{ row.listenHost || '' }}:{{ row.listenPort }}</span>
            <span v-else>{{ t('common.emDash') }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('portmap.client')" min-width="140" prop="clientAddr" />
        <el-table-column :label="t('portmap.sessionCol')" min-width="200" prop="sessionId" show-overflow-tooltip />
        <el-table-column :label="t('portmap.start')" min-width="160">
          <template #default="{ row }">{{ formatDateTime(row.openedAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('portmap.end')" min-width="160">
          <template #default="{ row }">{{ row.closedAt ? formatDateTime(row.closedAt) : t('common.running') }}</template>
        </el-table-column>
        <el-table-column :label="t('portmap.traffic')" min-width="140">
          <template #default="{ row }">
            ↓ {{ formatBytes(row.bytesIn) }} / ↑ {{ formatBytes(row.bytesOut) }}
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import AssetTreeSelect from '../../shared/AssetTreeSelect.vue'
import { formatBytes, formatDateTime } from '../../shared/format'

const { t } = useI18n()

const props = defineProps({
  assetId: { type: String, default: '' },
  assets: { type: Array, default: () => [] }
})

const loading = ref(false)
const creating = ref(false)
const rows = ref([])
const searchQuery = ref('')
const createVisible = ref(false)
const remarkVisible = ref(false)
const remarkSaving = ref(false)
const remarkForm = reactive({ id: '', text: '' })
const historyVisible = ref(false)
const historyLoading = ref(false)
const historyRows = ref([])
const historyTitle = ref('')

const form = reactive({
  assetId: '',
  direction: 'gateway_to_asset',
  protocol: 'tcp',
  targetHost: '127.0.0.1',
  targetPort: 22,
  listenHost: '127.0.0.1',
  listenPort: 0,
  remark: ''
})

const assetFilter = computed(() => props.assetId || '')
const fixedAsset = computed(() => !!props.assetId)
const assetOptions = computed(() => (props.assets?.length ? props.assets : null))

const filteredRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return rows.value
  return rows.value.filter((r) => {
    const listen = `${r.listenHost || (isReverse(r) ? '127.0.0.1' : '0.0.0.0')}:${r.listenPort}`
    const target = `${r.targetHost || ''}:${r.targetPort ?? ''}`
    return [
      r.assetDisplayName,
      r.hostname,
      r.groupName,
      r.assetId,
      r.protocol,
      r.remark,
      listen,
      target,
      r.targetHost,
      String(r.listenPort ?? ''),
      String(r.targetPort ?? '')
    ].some((v) => String(v || '').toLowerCase().includes(q))
  })
})

function isReverse(row) {
  return row?.direction === 'asset_to_gateway' || row?.direction === 'reverse'
}

function directionLabel(row) {
  return isReverse(row) ? t('portmap.dirAssetToServer') : t('portmap.dirServerToAsset')
}

function statusLabel(row) {
  if (row.listening) return t('portmap.listening')
  if (row.lastError) return t('portmap.bindFailed')
  if (isReverse(row) && row.online === false) return t('portmap.assetOffline')
  return t('portmap.notListening')
}

function statusTagType(row) {
  if (row.listening) return 'success'
  if (row.lastError) return 'danger'
  if (isReverse(row) && row.online === false) return 'info'
  return 'warning'
}

async function reload() {
  loading.value = true
  try {
    const params = assetFilter.value ? { assetId: assetFilter.value } : {}
    const { data } = await api.get('/port-mappings', { params })
    rows.value = data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.assetId = props.assetId || form.assetId || ''
  form.direction = 'gateway_to_asset'
  form.protocol = 'tcp'
  form.targetHost = '127.0.0.1'
  form.targetPort = 22
  form.listenHost = '127.0.0.1'
  form.listenPort = 0
  form.remark = ''
  createVisible.value = true
}

async function create() {
  const assetId = props.assetId || form.assetId
  if (!assetId) {
    ElMessage.warning(t('portmap.selectAssetWarn'))
    return
  }
  creating.value = true
  try {
    const body = {
      assetId,
      direction: form.direction,
      protocol: form.protocol,
      targetHost: form.targetHost,
      targetPort: form.targetPort,
      remark: form.remark
    }
    if (form.direction === 'asset_to_gateway') {
      body.listenHost = form.listenHost || '127.0.0.1'
      if (form.listenPort > 0) {
        body.listenPort = form.listenPort
      }
    }
    await api.post('/port-mappings', body)
    ElMessage.success(t('portmap.created'))
    createVisible.value = false
    await reload()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || t('common.createFailed'))
  } finally {
    creating.value = false
  }
}

function openEditRemark(row) {
  remarkForm.id = row.id
  remarkForm.text = row.remark || ''
  remarkVisible.value = true
}

async function saveRemark() {
  remarkSaving.value = true
  try {
    await api.patch(`/port-mappings/${remarkForm.id}`, { remark: remarkForm.text })
    ElMessage.success(t('portmap.remarkUpdated'))
    remarkVisible.value = false
    await reload()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || t('common.saveFailed'))
  } finally {
    remarkSaving.value = false
  }
}

async function remove(row) {
  const dir = directionLabel(row)
  const listen = `${row.listenHost || ''}:${row.listenPort}`
  try {
    await ElMessageBox.confirm(
      t('portmap.deleteConfirm', {
        dir,
        proto: row.protocol?.toUpperCase(),
        listen,
        target: `${row.targetHost}:${row.targetPort}`
      }),
      t('portmap.deleteTitle'),
      { type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await api.delete(`/port-mappings/${row.id}`)
    ElMessage.success(t('common.deleteSuccess'))
    await reload()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || t('common.deleteFailed'))
  }
}

async function openHistory(row) {
  historyTitle.value = t('portmap.historyTitleDetail', {
    dir: directionLabel(row),
    port: row.listenPort,
    target: `${row.targetHost}:${row.targetPort}`
  })
  historyVisible.value = true
  historyLoading.value = true
  try {
    const { data } = await api.get('/port-mappings/history', { params: { mappingId: row.id } })
    historyRows.value = data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || t('portmap.loadHistoryFailed'))
  } finally {
    historyLoading.value = false
  }
}

watch(() => props.assetId, () => reload())
onMounted(() => {
  historyTitle.value = t('portmap.historyTitle')
  reload()
})

defineExpose({ reload })
</script>

<style scoped>
.toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.toolbar-search { flex: 1; max-width: 360px; min-width: 180px; }
.toolbar-actions { display: flex; gap: 8px; align-items: center; flex-shrink: 0; margin-left: auto; }
.hint { color: #909399; font-size: 12px; }
.sub { color: #909399; font-size: 12px; }
.err-hint { color: #e6a23c; font-size: 12px; margin-top: 8px; }
.form-tip { color: #909399; font-size: 12px; margin: 0 0 0 108px; line-height: 1.5; }
.inline-hint { margin-left: 8px; color: #909399; font-size: 12px; }
</style>
