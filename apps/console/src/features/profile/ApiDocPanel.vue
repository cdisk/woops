<template>
  <div class="api-doc">
    <div class="doc-layout">
      <div class="doc-main">
        <div class="toolbar">
          <p class="hint">{{ t('profile.apiDocHint') }}</p>
        </div>

        <!-- Visual replacement for {{API_REQUEST_PREFIX}} in the source Markdown. -->
        <div class="base-bar">
          <div class="base-row">
            <span class="base-label">{{ t('profile.apiBaseLabel') }}</span>
            <code class="base-value" :title="apiBase">{{ apiBase }}</code>
            <div class="base-actions">
              <el-button size="small" @click="copyBase">{{ t('profile.copyApiBase') }}</el-button>
              <el-button size="small" type="primary" plain @click="copyMd">{{ t('profile.copyMarkdown') }}</el-button>
            </div>
          </div>
          <p class="base-hint">{{ t('profile.apiBaseHint') }}</p>
        </div>

        <nav v-if="toc.length && !fixedToc" class="toc toc-inline" aria-label="toc">
          <div class="toc-title">{{ t('profile.apiDocToc') }}</div>
          <div class="toc-links">
            <a
              v-for="item in toc"
              :key="'m-' + item.id"
              class="toc-link"
              :class="[`lvl-${item.level}`, { active: item.id === activeId }]"
              href="#"
              @click.prevent="scrollTo(item.id)"
            >{{ item.text }}</a>
          </div>
        </nav>

        <div ref="bodyRef" class="md-body" v-html="html" />
      </div>
      <div ref="tocSlotRef" class="toc-slot" aria-hidden="true" />
    </div>

    <Teleport to="body">
      <nav
        v-if="toc.length && fixedToc && tocReady"
        class="toc toc-fixed"
        :style="tocStyle"
        aria-label="toc"
      >
        <div class="toc-title">{{ t('profile.apiDocToc') }}</div>
        <div class="toc-links">
          <a
            v-for="item in toc"
            :key="item.id"
            class="toc-link"
            :class="[`lvl-${item.level}`, { active: item.id === activeId }]"
            href="#"
            @click.prevent="scrollTo(item.id)"
          >{{ item.text }}</a>
        </div>
      </nav>
    </Teleport>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import apiDocMd from './apiTokenDoc.md?raw'
import { renderApiDoc } from './renderApiDocMd'

const { t } = useI18n()
const DOC_PLACEHOLDER_BASE = 'https://ops.example.com'
const API_BASE_TOKEN = '{{API_REQUEST_PREFIX}}'

function envConsolePublicHttp() {
  try {
    // Injected by vite.config.js from OPS_CONSOLE_PUBLIC_HTTP (may be empty).
    return String(__WOOPS_CONSOLE_PUBLIC_HTTP__ || '').trim().replace(/\/$/, '')
  } catch {
    return ''
  }
}

function resolveApiBase() {
  const fromEnv = envConsolePublicHttp()
  if (fromEnv) return fromEnv
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin.replace(/\/$/, '')
  }
  return DOC_PLACEHOLDER_BASE
}

const apiBase = ref(resolveApiBase())
// The placeholder is represented by the interactive base bar above.
const previewMd = computed(() => apiDocMd.replace(API_BASE_TOKEN, ''))
const rendered = computed(() => renderApiDoc(previewMd.value))
const html = computed(() => rendered.value.html)
const toc = computed(() => rendered.value.toc.filter((x) => x.level >= 2))
const bodyRef = ref(null)
const tocSlotRef = ref(null)
const activeId = ref('')
const fixedToc = ref(true)
const tocReady = ref(false)
const tocStyle = ref({})
let scrollRoot = null
let observer = null
let resizeObserver = null
let positionFrame = 0

function findScrollRoot(el) {
  let cur = el
  while (cur && cur !== document.body) {
    const style = getComputedStyle(cur)
    const oy = style.overflowY
    if ((oy === 'auto' || oy === 'scroll' || oy === 'overlay') && cur.scrollHeight > cur.clientHeight + 1) {
      return cur
    }
    cur = cur.parentElement
  }
  return window
}

function syncTocPosition() {
  const narrow = window.matchMedia('(max-width: 960px)').matches
  fixedToc.value = !narrow
  if (narrow || !tocSlotRef.value) {
    tocReady.value = false
    tocStyle.value = {}
    return
  }
  const rect = tocSlotRef.value.getBoundingClientRect()
  if (rect.width < 1 || rect.left < 1) {
    tocReady.value = false
    tocStyle.value = {}
    return
  }
  const top = 72
  tocStyle.value = {
    top: `${top}px`,
    left: `${Math.round(rect.left)}px`,
    width: `${Math.round(rect.width)}px`,
    maxHeight: `calc(100vh - ${top + 24}px)`
  }
  tocReady.value = true
}

function scrollTo(id) {
  const el = document.getElementById(id)
  if (!el) return
  activeId.value = id
  el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function setupObserver() {
  observer?.disconnect()
  const headings = toc.value
    .map((item) => document.getElementById(item.id))
    .filter(Boolean)
  if (!headings.length) return
  const root = scrollRoot === window ? null : scrollRoot
  observer = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((e) => e.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
      if (visible[0]?.target?.id) {
        activeId.value = visible[0].target.id
      }
    },
    {
      root,
      rootMargin: '-12% 0px -70% 0px',
      threshold: [0, 1]
    }
  )
  headings.forEach((h) => observer.observe(h))
  if (!activeId.value && toc.value[0]) activeId.value = toc.value[0].id
}

function onScrollOrResize() {
  cancelAnimationFrame(positionFrame)
  positionFrame = requestAnimationFrame(syncTocPosition)
}

function mdWithBase() {
  const base = apiBase.value || DOC_PLACEHOLDER_BASE
  const baseBlock = `**${t('profile.apiBaseLabel')}：** \`${base}\``
  return apiDocMd
    .replace(API_BASE_TOKEN, baseBlock)
    .split(DOC_PLACEHOLDER_BASE)
    .join(base)
}

async function copyText(text) {
  await navigator.clipboard.writeText(text)
}

async function copyBase() {
  try {
    await copyText(apiBase.value)
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.error(t('common.copyFailed'))
  }
}

async function copyMd() {
  try {
    await copyText(mdWithBase())
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.error(t('common.copyFailed'))
  }
}

onMounted(async () => {
  apiBase.value = resolveApiBase()
  await nextTick()
  scrollRoot = findScrollRoot(bodyRef.value)
  syncTocPosition()
  resizeObserver = new ResizeObserver(onScrollOrResize)
  if (tocSlotRef.value) resizeObserver.observe(tocSlotRef.value)
  if (bodyRef.value?.parentElement) resizeObserver.observe(bodyRef.value.parentElement)
  requestAnimationFrame(() => requestAnimationFrame(syncTocPosition))
  setupObserver()
  window.addEventListener('resize', onScrollOrResize)
  if (scrollRoot === window) {
    window.addEventListener('scroll', onScrollOrResize, { passive: true })
  } else {
    scrollRoot.addEventListener('scroll', onScrollOrResize, { passive: true })
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  resizeObserver?.disconnect()
  cancelAnimationFrame(positionFrame)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize)
  if (scrollRoot && scrollRoot !== window) {
    scrollRoot.removeEventListener('scroll', onScrollOrResize)
  }
})
</script>

<style scoped>
.toolbar {
  margin-bottom: 10px;
}
.hint {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
}
.base-bar {
  position: sticky;
  top: 0;
  z-index: 5;
  margin-bottom: 16px;
  padding: 10px 12px;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  background: var(--ops-bg-surface, #fff);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.02);
}
.base-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}
.base-label {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--ops-text-muted, var(--el-text-color-secondary));
}
.base-value {
  flex: 1;
  min-width: 0;
  font-family: var(--ops-font-mono), ui-monospace, monospace;
  font-size: 13px;
  padding: 4px 8px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.base-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}
.base-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.4;
}
.doc-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 32px;
  align-items: start;
}
.doc-main {
  min-width: 0;
  width: 100%;
}
.toc-slot {
  width: 220px;
  min-height: 1px;
  pointer-events: none;
}
.md-body {
  min-width: 0;
  width: 100%;
  font-size: 14px;
  line-height: 1.65;
  color: var(--ops-text);
}
.md-body :deep(h1),
.md-body :deep(h2),
.md-body :deep(h3) {
  scroll-margin-top: 88px;
}
.md-body :deep(h1) {
  font-size: 1.45rem;
  margin: 0 0 12px;
  font-weight: 600;
}
.md-body :deep(h2) {
  font-size: 1.15rem;
  margin: 1.4em 0 0.6em;
  font-weight: 600;
  border-bottom: 1px solid var(--ops-border);
  padding-bottom: 0.25em;
}
.md-body :deep(h3) {
  font-size: 1rem;
  margin: 1.1em 0 0.45em;
  font-weight: 600;
}
.md-body :deep(p) { margin: 0.55em 0; }
.md-body :deep(ul) { margin: 0.4em 0 0.6em; padding-left: 1.35em; }
.md-body :deep(li) { margin: 0.2em 0; }
.md-body :deep(code) {
  font-family: var(--ops-font-mono), ui-monospace, monospace;
  font-size: 0.9em;
  background: var(--el-fill-color-light);
  padding: 0.1em 0.35em;
  border-radius: 3px;
}
.md-body :deep(pre) {
  margin: 0.75em 0;
  padding: 12px 14px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  overflow-x: auto;
}
.md-body :deep(pre code) {
  background: none;
  padding: 0;
  font-size: 12.5px;
  line-height: 1.5;
}
.md-body :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 0.75em 0;
  font-size: 13px;
}
.md-body :deep(th),
.md-body :deep(td) {
  border: 1px solid var(--ops-border);
  padding: 6px 10px;
  text-align: left;
  vertical-align: top;
}
.md-body :deep(th) {
  background: var(--el-fill-color-lighter);
  font-weight: 600;
}
.md-body :deep(a) { color: var(--ops-color-primary); }

.toc-inline {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--ops-border);
  font-size: 12.5px;
}
.toc-inline .toc-links {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 6px;
}
.toc-inline .toc-link {
  margin-left: 0;
  border-left: none;
  padding-left: 0;
}
.toc-inline .toc-link.active {
  color: var(--ops-color-primary);
  text-decoration: underline;
}

@media (max-width: 960px) {
  .doc-layout {
    grid-template-columns: 1fr;
  }
  .toc-slot { display: none; }
}

.toc-title {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--ops-text-muted, var(--el-text-color-secondary));
  letter-spacing: 0.02em;
}
.toc-link {
  display: block;
  padding: 4px 0;
  color: var(--el-text-color-secondary);
  text-decoration: none;
  border-left: 2px solid transparent;
  margin-left: -15px;
  padding-left: 13px;
}
.toc-link.lvl-3 {
  padding-left: 24px;
}
.toc-link:hover {
  color: var(--ops-color-primary);
}
.toc-link.active {
  color: var(--ops-color-primary);
  border-left-color: var(--ops-color-primary);
  font-weight: 500;
}
</style>

<style>
.toc.toc-fixed {
  position: fixed;
  z-index: 20;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 4px 0 4px 14px;
  border-left: 1px solid var(--ops-border, #e5e7eb);
  font-size: 12.5px;
  line-height: 1.35;
  box-sizing: border-box;
  background: var(--ops-bg-page, #fff);
}
.toc.toc-fixed .toc-title {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--ops-text-muted, #6b7280);
  flex-shrink: 0;
}
.toc.toc-fixed .toc-links {
  min-height: 0;
  overflow-y: auto;
}
.toc.toc-fixed .toc-link {
  display: block;
  padding: 4px 0;
  color: var(--el-text-color-secondary, #6b7280);
  text-decoration: none;
  border-left: 2px solid transparent;
  margin-left: -15px;
  padding-left: 13px;
}
.toc.toc-fixed .toc-link.lvl-3 {
  padding-left: 24px;
}
.toc.toc-fixed .toc-link:hover {
  color: var(--ops-color-primary, #2f5d9f);
}
.toc.toc-fixed .toc-link.active {
  color: var(--ops-color-primary, #2f5d9f);
  border-left-color: var(--ops-color-primary, #2f5d9f);
  font-weight: 500;
}
</style>
