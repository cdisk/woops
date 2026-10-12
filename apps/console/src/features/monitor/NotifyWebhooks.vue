<template>
  <div class="page" v-loading="loading">
    <div class="page-toolbar">
      <el-button type="primary" size="small" @click="openCreate">{{ t('monitor.addWebhook') }}</el-button>
    </div>
    <el-table :data="rows" size="small" stripe>
      <el-table-column prop="name" :label="t('common.name')" min-width="140" />
      <el-table-column prop="url" :label="t('monitor.webhookUrl')" min-width="220" show-overflow-tooltip />
      <el-table-column :label="t('monitor.scope')" min-width="140">
        <template #default="{ row }">{{ scopeLabel(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.hasToken')" width="90">
        <template #default="{ row }">{{ row.hasToken ? t('common.yes') : t('common.no') }}</template>
      </el-table-column>
      <el-table-column :label="t('monitor.enabled')" width="90">
        <template #default="{ row }">
          <el-switch v-model="row.enabled" @change="(v) => toggle(row, v)" />
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="200" align="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="test(row)">{{ t('monitor.testWebhook') }}</el-button>
          <el-button link type="primary" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
          <el-button link type="danger" @click="remove(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="visible" :title="editing ? t('monitor.editWebhook') : t('monitor.createWebhook')" width="520px">
      <el-form label-width="100px">
        <el-form-item :label="t('common.name')">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('monitor.webhookUrl')">
          <el-input v-model="form.url" placeholder="https://..." />
        </el-form-item>
        <el-form-item :label="t('monitor.bearerToken')">
          <el-input
            v-model="form.bearerToken"
            type="password"
            show-password
            :placeholder="editing?.hasToken ? t('monitor.tokenKeep') : ''"
          />
        </el-form-item>
        <el-form-item :label="t('monitor.scope')">
          <el-select v-model="form.scopeType" style="width:140px" @change="onScopeType">
            <el-option :label="t('monitor.scopeAll')" value="ALL" />
            <el-option :label="t('monitor.scopeGroup')" value="GROUP" />
            <el-option :label="t('monitor.scopeAsset')" value="ASSET" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.scopeType === 'GROUP'" :label="t('monitor.scopeGroup')">
          <el-select v-model="form.scopeId" filterable style="width:100%">
            <el-option v-for="g in flatGroups" :key="g.id" :label="g.label" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.scopeType === 'ASSET'" :label="t('monitor.scopeAsset')">
          <AssetTreeSelect v-model="form.scopeId" width="100%" />
        </el-form-item>
        <el-form-item :label="t('monitor.enabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import AssetTreeSelect from '../../shared/AssetTreeSelect.vue'

const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const rows = ref([])
const groups = ref([])
const visible = ref(false)
const editing = ref(null)
const form = reactive({
  name: '',
  url: '',
  bearerToken: '',
  scopeType: 'ALL',
  scopeId: '',
  enabled: true
})

const flatGroups = computed(() => flattenGroups(groups.value))

function flattenGroups(nodes, depth = 0, out = []) {
  for (const n of nodes || []) {
    out.push({ id: n.id, label: `${'—'.repeat(depth)}${n.name}` })
    flattenGroups(n.children, depth + 1, out)
  }
  return out
}

function scopeLabel(row) {
  if (row.scopeType === 'ALL') return t('monitor.scopeAll')
  if (row.scopeType === 'GROUP') return `${t('monitor.scopeGroup')}: ${row.scopeId || ''}`
  if (row.scopeType === 'ASSET') return `${t('monitor.scopeAsset')}: ${row.scopeId || ''}`
  return row.scopeType || ''
}

async function load() {
  loading.value = true
  try {
    const [w, g] = await Promise.all([
      api.get('/monitor/notify/webhooks'),
      api.get('/groups')
    ])
    rows.value = w.data || []
    groups.value = g.data || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.url = ''
  form.bearerToken = ''
  form.scopeType = 'ALL'
  form.scopeId = ''
  form.enabled = true
  visible.value = true
}

function openEdit(row) {
  editing.value = row
  form.name = row.name
  form.url = row.url
  form.bearerToken = ''
  form.scopeType = row.scopeType || 'ALL'
  form.scopeId = row.scopeId || ''
  form.enabled = row.enabled
  visible.value = true
}

function onScopeType() {
  form.scopeId = ''
}

async function save() {
  saving.value = true
  try {
    const body = {
      name: form.name,
      url: form.url,
      enabled: form.enabled,
      scopeType: form.scopeType,
      scopeId: form.scopeType === 'ALL' ? null : form.scopeId || null
    }
    if (form.bearerToken) body.bearerToken = form.bearerToken
    if (editing.value) {
      await api.patch(`/monitor/notify/webhooks/${editing.value.id}`, body)
    } else {
      await api.post('/monitor/notify/webhooks', body)
    }
    visible.value = false
    ElMessage.success(t('common.saveSuccess'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function toggle(row, enabled) {
  try {
    await api.patch(`/monitor/notify/webhooks/${row.id}`, { enabled })
  } catch {
    row.enabled = !enabled
    ElMessage.error(t('monitor.updateFailed'))
  }
}

async function test(row) {
  try {
    await api.post(`/monitor/notify/webhooks/${row.id}/test`)
    ElMessage.success(t('monitor.testOk'))
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('monitor.testFailed'))
  }
}

async function remove(row) {
  await ElMessageBox.confirm(t('monitor.deleteWebhookConfirm', { name: row.name }), t('monitor.confirmTitle'))
  await api.delete(`/monitor/notify/webhooks/${row.id}`)
  ElMessage.success(t('common.deleteSuccess'))
  await load()
}

onMounted(load)
</script>

<style scoped>
.page-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
}
</style>
