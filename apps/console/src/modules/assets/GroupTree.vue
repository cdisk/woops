<template>
  <aside class="tree-pane">
    <div class="pane-title">{{ t('groups.title') }}</div>
    <el-tree
      ref="treeRef"
      :data="treeData"
      node-key="key"
      highlight-current
      default-expand-all
      :expand-on-click-node="false"
      :props="{ label: 'label', children: 'children' }"
      @node-click="onTreeClick"
    >
      <template #default="{ data }">
        <div class="tree-node">
          <span class="tree-label">{{ data.label }}</span>
          <span v-if="data.kind === 'all' && canCreateRootGroup" class="tree-actions" @click.stop>
            <button type="button" class="tree-link-btn" @click="createChild(null)">{{ t('groups.addGroup') }}</button>
          </span>
          <span
            v-else-if="data.kind === 'group' && canManageInventory() && data.manageable !== false"
            class="tree-actions"
            :class="{ 'is-open': !!openMenus[data.id] }"
            @click.stop
          >
            <button type="button" class="tree-link-btn" @click="createChild(data.id)">{{ t('groups.addGroup') }}</button>
            <el-dropdown
              trigger="click"
              @visible-change="(v) => setMenuOpen(data.id, v)"
              @command="(cmd) => onGroupCommand(cmd, data)"
            >
              <button type="button" class="tree-link-btn tree-caret" :aria-label="t('common.more')">
                <IconChevronDown :size="14" stroke="1.75" />
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="rename">{{ t('files.rename') }}</el-dropdown-item>
                  <el-dropdown-item command="delete" divided>{{ t('common.delete') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </span>
        </div>
      </template>
    </el-tree>
  </aside>
</template>

<script>
/** Selected key for the virtual "All" root node. */
export const ALL_KEY = '__all__'
</script>

<script setup>
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { IconChevronDown } from '@tabler/icons-vue'
import api from '../../shared/api'
import { canManageInventory, isSuperAdmin } from '../../shared/auth'

const props = defineProps({
  modelValue: { type: String, default: '__all__' }
})

const emit = defineEmits(['update:modelValue', 'change'])

const { t } = useI18n()

/** Same as exported ALL_KEY; local copy for setup logic (defineProps default cannot use locals). */
const ALL_KEY = '__all__'

const groupTree = ref([])
const treeRef = ref(null)
const openMenus = reactive({})
const canCreateRootGroup = computed(() => isSuperAdmin())

const treeData = computed(() => [
  {
    key: ALL_KEY,
    kind: 'all',
    label: t('groups.all'),
    children: mapGroups(groupTree.value)
  }
])

function setMenuOpen(id, open) {
  openMenus[id] = open
}

function mapGroups(nodes) {
  return (nodes || []).map((n) => ({
    key: n.id,
    id: n.id,
    kind: 'group',
    label: n.name,
    name: n.name,
    parentId: n.parentId || null,
    manageable: n.manageable !== false,
    children: mapGroups(n.children || [])
  }))
}

function findGroup(nodes, id) {
  for (const n of nodes || []) {
    if (n.id === id) return n
    const hit = findGroup(n.children || [], id)
    if (hit) return hit
  }
  return null
}

function select(key, notify) {
  emit('update:modelValue', key)
  if (notify) emit('change', key)
}

function onTreeClick(data) {
  select(data.key, true)
}

function onGroupCommand(cmd, data) {
  if (cmd === 'rename') renameGroup(data)
  else if (cmd === 'delete') deleteGroup(data)
}

async function loadGroups() {
  try {
    const { data } = await api.get('/groups')
    groupTree.value = data || []
  } catch (e) {
    groupTree.value = []
    ElMessage.error(e?.response?.data?.error || t('groups.loadFailed'))
  }
}

async function syncCurrentKey() {
  await nextTick()
  treeRef.value?.setCurrentKey(props.modelValue)
}

async function reload() {
  await loadGroups()
  await syncCurrentKey()
}

async function createChild(parentId) {
  try {
    const { value } = await ElMessageBox.prompt(t('groups.createPrompt'), t('groups.createTitle'), {
      confirmButtonText: t('common.create'),
      cancelButtonText: t('common.cancel'),
      inputPattern: /\S+/,
      inputErrorMessage: t('common.nameRequired')
    })
    const body = { name: value.trim() }
    if (parentId) body.parentId = parentId
    await api.post('/groups', body)
    ElMessage.success(t('common.createSuccess'))
    await reload()
    emit('change', props.modelValue)
  } catch {
    /* cancelled */
  }
}

async function renameGroup(data) {
  try {
    const { value } = await ElMessageBox.prompt(t('groups.renamePrompt'), t('groups.renameTitle'), {
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
      inputValue: data.name || data.label,
      inputPattern: /\S+/,
      inputErrorMessage: t('common.nameRequired')
    })
    await api.patch(`/groups/${data.id}`, { name: value.trim() })
    ElMessage.success(t('common.renameSuccess'))
    await reload()
    emit('change', props.modelValue)
  } catch {
    /* cancelled */
  }
}

async function deleteGroup(data) {
  try {
    await ElMessageBox.confirm(
      t('groups.deleteConfirm', { name: data.label }),
      t('groups.deleteTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }
  try {
    await api.delete(`/groups/${data.id}`)
    ElMessage.success(t('common.deleteSuccess'))
    const nextKey = props.modelValue === data.id ? ALL_KEY : props.modelValue
    if (nextKey !== props.modelValue) select(ALL_KEY, false)
    await reload()
    emit('change', nextKey)
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.deleteFailed'))
  }
}

watch(() => props.modelValue, () => {
  syncCurrentKey()
})

defineExpose({
  reload,
  findGroup: (id) => findGroup(groupTree.value, id)
})
</script>

<style scoped>
.tree-pane {
  width: 280px; flex-shrink: 0; border: 1px solid var(--ops-border); border-radius: var(--ops-radius);
  background: var(--ops-bg-surface);
  padding: 8px; overflow: auto;
}
.pane-title {
  font-weight: 600; margin-bottom: 8px; font-size: 13px;
}
.tree-node { display: flex; align-items: center; justify-content: space-between; width: 100%; padding-right: 4px; gap: 4px; }
.tree-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.tree-actions {
  display: none; flex-shrink: 0; align-items: center; margin-left: 4px; gap: 0;
}
.tree-node:hover .tree-actions,
.tree-actions.is-open { display: inline-flex; }
.tree-actions :deep(.el-dropdown) { display: inline-flex; line-height: 1; }
.tree-link-btn {
  appearance: none; border: none; background: transparent; box-shadow: none;
  color: var(--el-color-primary); font-size: 12px; line-height: 1; padding: 2px 4px;
  cursor: pointer; display: inline-flex; align-items: center;
}
.tree-link-btn:hover { color: var(--el-color-primary-light-3); }
.tree-caret { padding: 2px 2px; color: var(--ops-text-secondary); }
.tree-caret:hover { color: var(--el-color-primary); }
</style>
