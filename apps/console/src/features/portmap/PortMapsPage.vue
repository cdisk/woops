<template>
  <div class="page">
    <p class="hint">{{ t('portmap.pageDesc') }}</p>
    <PortMapPanel />
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import PortMapPanel from './PortMapPanel.vue'

const { t } = useI18n()
</script>

<style scoped>
.hint {
  margin: 0;
}
</style>
