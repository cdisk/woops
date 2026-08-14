<template>
  <TextFileEditor
    ref="editorRef"
    v-model="visible"
    v-model:charset="charset"
    :path="path"
    :file-name="name"
    :language="language"
    :initial-text="text"
    :loading="loading"
    :saving="saving"
    @save="save"
  />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import TextFileEditor from './TextFileEditor.vue'
import { downloadBytes, uploadBytes } from '../filetransfer/transferClient'
import {
  editorLanguageFor,
  isEditableTextFile,
  looksBinary,
  maxEditBytes
} from '../filetransfer/textFiles'
import { charsetLabel, decodeText, detectCharset, encodeText } from '../filetransfer/charset'
import { formatSize } from './fileFormat'

const { t } = useI18n()

const props = defineProps({
  assetId: { type: String, default: '' }
})
const emit = defineEmits(['saved', 'update:active'])

const editorRef = ref(null)
const visible = ref(false)
const loading = ref(false)
const saving = ref(false)
const path = ref('')
const name = ref('')
const language = ref('plain')
const text = ref('')
const charset = ref('utf-8')
// Kept so switching the charset re-decodes locally instead of re-reading the file.
let rawBytes = null
let hasBom = false
let decodedAs = ''

// Lets the host page disable file operations while an edit is in flight.
const active = computed(() => visible.value || loading.value || saving.value)
watch(active, (v) => emit('update:active', v))

watch(charset, (next) => {
  if (!rawBytes || next === decodedAs) return
  const detected = detectCharset(rawBytes)
  hasBom = detected.charset === next ? detected.bom : false
  decodedAs = next
  text.value = decodeText(rawBytes, next)
})

/** Open a remote text file: a short-lived filetransfer read into memory. */
async function open({ path: remotePath, name: fileName, size }) {
  if (!remotePath || !isEditableTextFile(fileName)) return
  if ((Number(size) || 0) > maxEditBytes()) {
    ElMessage.warning(t('files.fileTooLargeEdit', { size: formatSize(maxEditBytes()) }))
    return
  }
  path.value = remotePath
  name.value = fileName
  language.value = editorLanguageFor(fileName)
  text.value = ''
  rawBytes = null
  visible.value = true
  loading.value = true
  try {
    const bytes = await downloadBytes({
      assetId: props.assetId,
      remotePath,
      maxBytes: maxEditBytes()
    })
    if (bytes.length > maxEditBytes()) {
      visible.value = false
      ElMessage.warning(t('files.fileTooLargeEdit', { size: formatSize(maxEditBytes()) }))
      return
    }
    const detected = detectCharset(bytes)
    if (looksBinary(bytes, detected.charset)) {
      visible.value = false
      ElMessage.warning(t('files.binaryCannotEdit'))
      return
    }
    rawBytes = bytes
    hasBom = detected.bom
    decodedAs = detected.charset
    charset.value = detected.charset
    text.value = decodeText(bytes, detected.charset)
  } catch (e) {
    visible.value = false
    ElMessage.error(e.message || t('files.readFailed'))
  } finally {
    loading.value = false
  }
}

/** Save back through a one-shot upload (atomic replace happens on the agent). */
async function save(newText) {
  if (!path.value) return
  let target = charset.value
  let encoded = encodeText(newText, target, hasBom)
  if (encoded.unmapped.length) {
    const sample = encoded.unmapped.slice(0, 8).join(' ')
    try {
      await ElMessageBox.confirm(
        t('files.charsetIncompatible', {
          n: encoded.unmapped.length,
          charset: charsetLabel(target),
          sample
        }),
        t('files.charsetIncompatibleTitle'),
        {
          type: 'warning',
          confirmButtonText: t('files.saveAsUtf8'),
          cancelButtonText: t('files.cancelSave')
        }
      )
    } catch {
      return
    }
    target = 'utf-8'
    encoded = encodeText(newText, target, false)
  }
  saving.value = true
  try {
    if (encoded.bytes.length > maxEditBytes()) {
      ElMessage.warning(t('files.contentTooLarge', { size: formatSize(maxEditBytes()) }))
      return
    }
    await uploadBytes({
      assetId: props.assetId,
      remotePath: path.value,
      bytes: encoded.bytes
    })
    rawBytes = encoded.bytes
    if (target !== charset.value) {
      hasBom = false
      decodedAs = target
      charset.value = target
    }
    editorRef.value?.markSaved?.(newText)
    text.value = newText
    ElMessage.success(t('files.savedWithCharset', { charset: charsetLabel(target) }))
    emit('saved')
  } catch (e) {
    ElMessage.error(e.message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

defineExpose({ open })
</script>
