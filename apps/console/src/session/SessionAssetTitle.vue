<template>
  <div class="session-titles">
    <div class="title-line">{{ title }}</div>
    <div v-if="subtitle" class="sub-line" :title="subtitle">{{ subtitle }}</div>
  </div>
</template>

<script setup>
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { setSessionDocumentTitle } from './sessionTabTitle'

const props = defineProps({
  /** Primary line, e.g. "host-a · Shell" */
  title: { type: String, required: true },
  /** Protocol label for browser tab, e.g. "Shell" / "RDP" → "Shell · host · 10.0.0.5" */
  kind: { type: String, required: true },
  /** Asset summary from ticket `data.asset` */
  asset: { type: Object, default: null }
})

const { t } = useI18n()

const subtitle = computed(() => {
  const a = props.asset
  if (!a) return ''
  const group = a.groupId ? (a.groupName || '-') : t('common.ungrouped')
  const pub = a.publicIp || '-'
  const priv = a.privateIp || '-'
  return t('session.metaLine', { group, publicIp: pub, privateIp: priv })
})

watch(
  () => [props.kind, props.asset?.displayName, props.asset?.hostname, props.asset?.privateIp],
  () => setSessionDocumentTitle(props.kind, props.asset),
  { immediate: true }
)
</script>

<style scoped>
.session-titles {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.title-line {
  font-weight: 600;
  font-size: 14px;
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ops-text);
}
.sub-line {
  font-size: 11px;
  color: var(--ops-text-secondary);
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--ops-font-mono);
}
</style>
