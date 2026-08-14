<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('home.ignoredAlerts')"
    width="960px"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <p class="hint">{{ t('home.ignoredHint') }}</p>
    <el-table :data="rows" size="small" stripe :empty-text="t('home.emptyIgnored')" max-height="420">
      <el-table-column prop="displayName" :label="t('common.name')" min-width="140" />
      <el-table-column prop="hostname" :label="t('home.hostname')" min-width="120" />
      <el-table-column :label="t('home.ignoredItem')" min-width="160">
        <template #default="{ row }">{{ itemLabel(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('home.ignoredAt')" width="170">
        <template #default="{ row }">{{ formatTime(row.ignoredAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="120" align="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="unignore(row)">{{ t('home.unignore') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { formatTime } from '../../shared/format'

defineProps({
  modelValue: { type: Boolean, default: false },
  rows: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:modelValue', 'changed'])
const { t } = useI18n()

function itemLabel(row) {
  if (row.kind === 'offline' || row.itemId === 'host.online') return t('common.offline')
  return row.itemName || row.itemId
}

async function unignore(row) {
  try {
    await ElMessageBox.confirm(
      t('home.unignoreConfirm', { name: row.displayName || row.hostname, item: itemLabel(row) }),
      t('home.unignoreTitle')
    )
  } catch {
    return
  }
  try {
    await api.delete(`/assets/${row.assetId}/alert-ignores`, { params: { itemId: row.itemId } })
    ElMessage.success(t('home.unignoreSuccess'))
    emit('changed')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('home.unignoreFailed'))
  }
}
</script>

<style scoped>
.hint {
  margin: 0 0 12px;
  color: var(--ops-text-secondary);
  font-size: 13px;
}
</style>
