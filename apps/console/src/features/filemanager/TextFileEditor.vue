<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="90%"
    top="4vh"
    class="text-editor-dialog"
    :close-on-click-modal="false"
    :destroy-on-close="true"
    @update:model-value="onVisible"
  >
    <div v-loading="loading" class="editor-wrap">
      <div ref="host" class="cm-host"></div>
    </div>
    <template #footer>
      <div class="editor-foot">
        <span class="meta">{{ meta }}</span>
        <div class="actions">
          <el-select
            :model-value="charset"
            size="small"
            class="charset-select"
            :disabled="loading || saving"
            @change="onCharsetChange"
          >
            <el-option
              v-for="opt in CHARSET_OPTIONS"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
          <el-button
            :disabled="loading || saving"
            :title="t('files.searchHint')"
            @click="openSearch"
          >{{ t('files.search') }}</el-button>
          <el-button :disabled="loading || saving" @click="close">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" :disabled="loading || !dirty" @click="save">
            {{ t('files.saveAndUpload') }}
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, drawSelection } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { search, searchKeymap, highlightSelectionMatches, openSearchPanel } from '@codemirror/search'
import { StreamLanguage, syntaxHighlighting, defaultHighlightStyle, bracketMatching } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { CHARSET_OPTIONS, charsetLabel } from '../filetransfer/charset'

const { t } = useI18n()

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  path: { type: String, default: '' },
  fileName: { type: String, default: '' },
  language: { type: String, default: 'plain' }, // plain | shell
  initialText: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  charset: { type: String, default: 'utf-8' }
})

const emit = defineEmits(['update:modelValue', 'update:charset', 'save'])

const host = ref(null)
const dirty = ref(false)
let view = null
let baseline = ''

const dialogTitle = computed(() => (
  props.fileName ? t('files.editWithName', { name: props.fileName }) : t('files.editFile')
))
const meta = computed(() => {
  const lang = props.language === 'shell' ? t('files.langShell') : t('files.langPlain')
  const mark = dirty.value ? t('files.dirty') : t('files.clean')
  return `${lang} · ${charsetLabel(props.charset)} · ${mark}`
})

/** Re-decoding replaces the buffer, so unsaved edits would be dropped. */
async function onCharsetChange(next) {
  if (next === props.charset) return
  if (dirty.value) {
    try {
      await ElMessageBox.confirm(
        t('files.switchCharsetMsg'),
        t('files.switchCharsetTitle'),
        {
          type: 'warning',
          confirmButtonText: t('files.reload'),
          cancelButtonText: t('common.cancel')
        }
      )
    } catch {
      return
    }
  }
  emit('update:charset', next)
}

function languageExt() {
  if (props.language === 'shell') {
    return [
      StreamLanguage.define(shell),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true })
    ]
  }
  return [syntaxHighlighting(defaultHighlightStyle, { fallback: true })]
}

function destroyEditor() {
  if (view) {
    view.destroy()
    view = null
  }
}

function mountEditor(text) {
  destroyEditor()
  if (!host.value) return
  baseline = text
  dirty.value = false
  const state = EditorState.create({
    doc: text,
    extensions: [
      lineNumbers(),
      highlightActiveLine(),
      drawSelection(),
      history(),
      bracketMatching(),
      search({ top: true }),
      highlightSelectionMatches(),
      keymap.of([indentWithTab, ...searchKeymap, ...defaultKeymap, ...historyKeymap]),
      ...languageExt(),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) dirty.value = u.state.doc.toString() !== baseline
      }),
      EditorView.theme({
        '&': { height: '100%', fontSize: '13px' },
        '.cm-scroller': { overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' },
        '.cm-content': { minHeight: '100%' }
      }),
      EditorView.lineWrapping
    ]
  })
  view = new EditorView({ state, parent: host.value })
}

async function onVisible(v) {
  emit('update:modelValue', v)
  if (!v) {
    destroyEditor()
    dirty.value = false
  }
}

function close() {
  emit('update:modelValue', false)
}

function openSearch() {
  if (view) openSearchPanel(view)
}

function save() {
  if (!view) return
  const text = view.state.doc.toString()
  emit('save', text)
}

function markSaved(text) {
  baseline = text
  dirty.value = false
}

watch(
  () => [props.modelValue, props.loading, props.initialText, props.language],
  async ([open, loading]) => {
    if (!open) return
    if (loading) return
    await nextTick()
    mountEditor(props.initialText ?? '')
  }
)

onBeforeUnmount(() => destroyEditor())

defineExpose({ markSaved })
</script>

<style scoped>
.editor-wrap {
  height: min(72vh, 720px);
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
}
.cm-host { height: 100%; }
.editor-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
.meta { color: #64748b; font-size: 12px; }
.actions { display: flex; gap: 8px; align-items: center; }
.charset-select { width: 150px; }
</style>
