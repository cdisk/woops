<template>
  <el-tree-select
    :model-value="modelValue || undefined"
    :data="treeData"
    :props="treeProps"
    :placeholder="placeholder"
    :clearable="clearable"
    :filterable="filterable"
    :filter-node-method="filterNode"
    :size="size"
    :disabled="disabled"
    check-strictly
    default-expand-all
    :render-after-expand="false"
    :loading="loading"
    class="asset-tree-select"
    :style="width ? { width } : undefined"
    @update:model-value="onUpdate"
  />
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from './api'
import { buildAssetSelectTree, matchAssetTreeNode } from './assetTree'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  clearable: { type: Boolean, default: true },
  filterable: { type: Boolean, default: true },
  size: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  width: { type: String, default: '' },
  /** When set, use these assets instead of fetching (still loads groups). */
  assets: { type: Array, default: null },
  showOnline: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'change'])

const { t } = useI18n()
const loading = ref(false)
const groups = ref([])
const loadedAssets = ref([])

const treeProps = {
  label: 'label',
  value: 'value',
  children: 'children',
  disabled: 'disabled'
}

const effectiveAssets = computed(() =>
  Array.isArray(props.assets) ? props.assets : loadedAssets.value
)

const treeData = computed(() =>
  buildAssetSelectTree(groups.value, effectiveAssets.value, {
    ungroupedLabel: t('common.ungrouped'),
    showOnline: props.showOnline,
    onlineLabel: t('common.online'),
    offlineLabel: t('common.offline')
  })
)

function filterNode(query, data) {
  return matchAssetTreeNode(query, data)
}

function onUpdate(val) {
  const next = val == null || val === '' ? '' : String(val)
  // Ignore accidental group keys (disabled nodes should not emit).
  if (next.startsWith('group:')) return
  emit('update:modelValue', next)
  emit('change', next)
}

async function loadGroups() {
  try {
    const { data } = await api.get('/groups')
    groups.value = data || []
  } catch {
    groups.value = []
  }
}

async function loadAssets() {
  if (Array.isArray(props.assets)) return
  try {
    const { data } = await api.get('/assets')
    loadedAssets.value = data || []
  } catch {
    loadedAssets.value = []
  }
}

async function reload() {
  loading.value = true
  try {
    await Promise.all([loadGroups(), loadAssets()])
  } finally {
    loading.value = false
  }
}

watch(
  () => props.assets,
  () => {
    if (!Array.isArray(props.assets)) loadAssets()
  }
)

onMounted(reload)

defineExpose({ reload })
</script>

<style scoped>
.asset-tree-select {
  min-width: 180px;
}
</style>
