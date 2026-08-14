<template>
  <div class="shell-commands">
    <el-dropdown trigger="click" :disabled="!assetId" @command="onCommand">
      <el-button size="small">
        {{ t('shell.commands') }}
        <IconChevronDown :size="14" stroke="1.75" class="caret" />
      </el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item v-if="!items.length" disabled>{{ t('shell.noCommands') }}</el-dropdown-item>
          <el-dropdown-item
            v-for="(cmd, idx) in items"
            :key="idx"
            :command="{ type: 'run', index: idx }"
            :title="cmd"
          >
            {{ labelOf(cmd) }}
          </el-dropdown-item>
          <el-dropdown-item divided :command="{ type: 'manage' }">{{ t('shell.manage') }}</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>

    <el-dialog
      v-model="dialogVisible"
      :title="t('shell.commandsTitle')"
      width="560px"
      align-center
      :close-on-click-modal="false"
      @closed="onDialogClosed"
    >
      <p class="hint">{{ t('shell.commandsHint') }}</p>
      <div v-if="!draft.length" class="empty">{{ t('shell.commandsEmpty') }}</div>
      <div v-for="(row, idx) in draft" :key="idx" class="row">
        <el-input
          v-model="draft[idx]"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="t('shell.commandPlaceholder')"
        />
        <el-button type="danger" link @click="removeAt(idx)">{{ t('common.delete') }}</el-button>
      </div>
      <template #footer>
        <el-button :disabled="draft.length >= MAX_COUNT || saving" @click="addRow">{{ t('shell.add') }}</el-button>
        <el-button :disabled="saving" @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { IconChevronDown } from '@tabler/icons-vue'
import api from '../../shared/api'

const { t } = useI18n()
const MAX_COUNT = 30
const LABEL_MAX = 40

const props = defineProps({
  assetId: { type: String, required: true }
})
const emit = defineEmits(['run'])

const items = ref([])
const dialogVisible = ref(false)
const draft = ref([])
const saving = ref(false)

function labelOf(cmd) {
  const first = String(cmd || '').split('\n').find((l) => l.trim()) || ''
  const line = first.trim()
  if (line.length <= LABEL_MAX) return line || t('shell.emptyLabel')
  return `${line.slice(0, LABEL_MAX)}…`
}

async function load() {
  if (!props.assetId) {
    items.value = []
    return
  }
  try {
    const { data } = await api.get(`/assets/${props.assetId}/shell-commands`)
    items.value = Array.isArray(data?.items) ? data.items.map(String) : []
  } catch (e) {
    const msg = e.response?.data?.error || e.response?.data?.message || e.message || t('shell.loadCommandsFailed')
    ElMessage.error(msg)
  }
}

function onCommand(cmd) {
  if (!cmd || typeof cmd !== 'object') return
  if (cmd.type === 'manage') {
    draft.value = items.value.map((s) => s)
    if (!draft.value.length) draft.value = ['']
    dialogVisible.value = true
    return
  }
  if (cmd.type === 'run' && Number.isInteger(cmd.index)) {
    const text = items.value[cmd.index]
    if (text) emit('run', text)
  }
}

function addRow() {
  if (draft.value.length >= MAX_COUNT) return
  draft.value.push('')
}

function removeAt(idx) {
  draft.value.splice(idx, 1)
}

function onDialogClosed() {
  draft.value = []
}

async function save() {
  const next = draft.value.map((s) => String(s ?? '').replace(/\r\n/g, '\n').replace(/\r/g, '\n'))
  if (next.some((s) => !s.trim())) {
    ElMessage.warning(t('shell.fillOrRemove'))
    return
  }
  if (next.length > MAX_COUNT) {
    ElMessage.warning(t('shell.maxCount', { n: MAX_COUNT }))
    return
  }
  saving.value = true
  try {
    const { data } = await api.put(`/assets/${props.assetId}/shell-commands`, { items: next })
    items.value = Array.isArray(data?.items) ? data.items.map(String) : next
    dialogVisible.value = false
    ElMessage.success(t('common.saveSuccess'))
  } catch (e) {
    const msg = e.response?.data?.error || e.response?.data?.message || e.message || t('common.saveFailed')
    ElMessage.error(msg)
  } finally {
    saving.value = false
  }
}

watch(() => props.assetId, () => { load() })
onMounted(() => { load() })
</script>

<style scoped>
.shell-commands { display: inline-flex; align-items: center; }
.caret { margin-left: 2px; vertical-align: -2px; }
.hint { margin: 0 0 12px; line-height: 1.5; color: var(--el-text-color-secondary); font-size: 13px; }
.empty { margin-bottom: 12px; color: var(--el-text-color-secondary); font-size: 13px; }
.row { display: flex; gap: 8px; align-items: flex-start; margin-bottom: 10px; }
.row .el-input { flex: 1; min-width: 0; }
.row .el-button { margin-top: 4px; flex-shrink: 0; }
</style>
