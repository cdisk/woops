<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('assets.setUpstreamTitle')"
    width="520px"
    align-center
    @close="emit('update:modelValue', false)"
  >
    <p class="hint">{{ t('assets.setUpstreamHint') }}</p>
    <p class="picked">{{ t('assets.selectedAssets', { n: assets.length }) }}</p>
    <ul class="names">
      <li v-for="asset in assets" :key="asset.id">{{ asset.displayName || asset.hostname || asset.id }}</li>
    </ul>
    <AssetTreeSelect
      ref="pickerRef"
      v-model="upstreamId"
      show-online
      width="100%"
      :exclude-ids="excludedIds"
      :placeholder="t('assets.upstreamPlaceholder')"
    />
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">{{ t('common.cancel') }}</el-button>
      <el-button :loading="saving" @click="save(true)">{{ t('assets.clearUpstream') }}</el-button>
      <el-button type="primary" :loading="saving" @click="save(false)">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import api from '../../shared/api'
import AssetTreeSelect from '../../shared/AssetTreeSelect.vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  assets: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue', 'saved'])

const { t } = useI18n()
const saving = ref(false)
const upstreamId = ref('')
const pickerRef = ref(null)
const excludedIds = computed(() => props.assets.map((asset) => asset.id))

watch(() => props.modelValue, (open) => {
  if (!open) return
  upstreamId.value = ''
  pickerRef.value?.reload()
})

async function save(clear) {
  if (!props.assets.length) return
  if (!clear && !upstreamId.value) {
    ElMessage.warning(t('assets.upstreamRequired'))
    return
  }
  saving.value = true
  try {
    await api.put('/assets/update-upstream', {
      assetIds: props.assets.map((asset) => asset.id),
      upstreamAssetId: clear ? null : upstreamId.value,
      clear
    })
    ElMessage.success(clear ? t('assets.upstreamCleared') : t('assets.upstreamSaved'))
    emit('saved')
    emit('update:modelValue', false)
  } catch (error) {
    ElMessage.error(error?.response?.data?.error || error?.message || t('assets.upstreamSaveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.hint { margin: 0 0 8px; color: var(--ops-text-secondary); font-size: 13px; line-height: 1.5; }
.picked { margin: 0 0 6px; font-size: 13px; font-weight: 600; }
.names {
  margin: 0 0 12px;
  padding-left: 18px;
  max-height: 120px;
  overflow: auto;
  color: var(--ops-text);
  font-size: 13px;
}
</style>
