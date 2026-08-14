<template>
  <div class="page">
    <AssetMonitorPanel v-if="assetId" :asset-id="assetId" :asset="asset" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import api from '../../shared/api'
import AssetMonitorPanel from './AssetMonitorPanel.vue'

const { t } = useI18n()
const route = useRoute()
const assetId = computed(() => route.params.id)
const asset = ref(null)

onMounted(async () => {
  try {
    const { data } = await api.get(`/assets/${assetId.value}`)
    asset.value = data
  } catch {
    asset.value = { displayName: t('monitor.title') }
  }
})
</script>

<style scoped>
.page {
  padding: 16px 20px;
  min-height: 100vh;
  box-sizing: border-box;
  background: var(--ops-bg-page);
}
</style>
