<template>
  <div class="page" v-loading="loading">
    <div v-if="canManageAlertRules()" class="page-toolbar">
      <el-button type="primary" size="small" @click="openCreate">{{ t('monitor.addRule') }}</el-button>
    </div>
    <el-table :data="rules" size="small" stripe>
      <el-table-column prop="name" :label="t('common.name')" min-width="140" />
      <el-table-column prop="itemId" :label="t('monitor.itemId')" min-width="160" />
      <el-table-column prop="instance" :label="t('monitor.instance')" width="120">
        <template #default="{ row }">{{ row.instance || t('monitor.instanceAll') }}</template>
      </el-table-column>
      <el-table-column prop="op" :label="t('monitor.condition')" width="80" />
      <el-table-column prop="threshold" :label="t('monitor.threshold')" width="100" />
      <el-table-column :label="t('monitor.enabled')" width="90">
        <template #default="{ row }">
          <el-switch
            v-model="row.enabled"
            :disabled="!canManageAlertRules()"
            @change="(v) => toggle(row, v)"
          />
        </template>
      </el-table-column>
      <el-table-column v-if="canManageAlertRules()" :label="t('common.actions')" width="140" align="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
          <el-button link type="danger" @click="remove(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="visible" :title="editing ? t('monitor.editRule') : t('monitor.createRule')" width="480px">
      <el-form label-width="90px">
        <el-form-item :label="t('common.name')">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('monitor.itemId')">
          <el-select v-model="form.itemId" filterable style="width:100%">
            <el-option
            v-for="it in items"
            :key="it.itemId"
            :label="`${monitorItemLabel(it.itemId, it.name)} (${it.itemId})`"
            :value="it.itemId"
          />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('monitor.instance')">
          <el-input v-model="form.instance" :placeholder="t('monitor.instancePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('monitor.condition')">
          <el-select v-model="form.op" style="width:120px">
            <el-option label=">" value="gt" />
            <el-option label=">=" value="gte" />
            <el-option label="<" value="lt" />
            <el-option label="<=" value="lte" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('monitor.threshold')">
          <el-input-number v-model="form.threshold" :step="1" style="width:100%" />
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
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { canManageAlertRules } from '../../shared/auth'
import { monitorItemLabel } from './monitorItemLabel'

const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const rules = ref([])
const items = ref([])
const visible = ref(false)
const editing = ref(null)
const form = reactive({
  name: '',
  itemId: 'cpu.usage_percent',
  instance: '',
  op: 'gt',
  threshold: 90,
  enabled: true
})

async function load() {
  loading.value = true
  try {
    const [r, i] = await Promise.all([
      api.get('/monitor/alert-rules'),
      api.get('/monitor/items')
    ])
    rules.value = r.data || []
    items.value = i.data || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.itemId = 'cpu.usage_percent'
  form.instance = ''
  form.op = 'gt'
  form.threshold = 90
  form.enabled = true
  visible.value = true
}

function openEdit(row) {
  editing.value = row
  form.name = row.name
  form.itemId = row.itemId
  form.instance = row.instance || ''
  form.op = row.op
  form.threshold = row.threshold
  form.enabled = row.enabled
  visible.value = true
}

async function save() {
  saving.value = true
  try {
    const body = {
      name: form.name,
      itemId: form.itemId,
      instance: form.instance,
      op: form.op,
      threshold: form.threshold,
      enabled: form.enabled
    }
    if (editing.value) {
      await api.patch(`/monitor/alert-rules/${editing.value.id}`, body)
    } else {
      await api.post('/monitor/alert-rules', body)
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
    await api.patch(`/monitor/alert-rules/${row.id}`, { enabled })
  } catch {
    row.enabled = !enabled
    ElMessage.error(t('monitor.updateFailed'))
  }
}

async function remove(row) {
  await ElMessageBox.confirm(t('monitor.deleteConfirm', { name: row.name }), t('monitor.confirmTitle'))
  await api.delete(`/monitor/alert-rules/${row.id}`)
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
