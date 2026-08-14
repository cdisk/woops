<template>
  <div class="files-page">
    <div class="bar">
      <el-button @click="goBack">
        <IconX :size="16" stroke="1.75" />
        {{ t('common.close') }}
      </el-button>
      <SessionAssetTitle :title="title" :kind="t('files.kind')" :asset="assetInfo" />
      <el-tag v-if="platform" size="small">{{ platformLabel }}</el-tag>
      <el-tag size="small" :type="statusTag">{{ status }}</el-tag>
    </div>

    <div v-if="error" class="error-box">
      <el-result icon="warning" :title="t('files.unavailable')" :sub-title="error">
        <template #extra>
          <el-button type="primary" @click="goBack">
            <IconX :size="16" stroke="1.75" />
            {{ t('files.closeTab') }}
          </el-button>
        </template>
      </el-result>
    </div>

    <div v-else class="files-body">
      <SessionConnectingMask
        :visible="showBusyMask"
        :text="busyMaskText"
        :detail="busyMaskDetail"
      />
      <div class="path-bar">
        <el-input v-model="path" :placeholder="pathPlaceholder" @keyup.enter="refresh">
          <template #prepend>{{ t('files.path') }}</template>
        </el-input>
        <el-button :disabled="!ready || atRoots || dirLoading" @click="goUp">
          <IconArrowUp :size="16" stroke="1.75" />
          {{ t('files.parent') }}
        </el-button>
        <el-button :disabled="!ready || dirLoading" @click="refresh">
          <IconRefresh :size="16" stroke="1.75" />
          {{ t('common.refresh') }}
        </el-button>
        <el-button :disabled="!ready || uploading || atRoots || dirLoading" @click="pickFiles">
          <IconUpload :size="16" stroke="1.75" />
          {{ t('files.uploadFile') }}
        </el-button>
        <el-button :disabled="!ready || uploading || atRoots || dirLoading" @click="pickFolder">
          <IconFolderUp :size="16" stroke="1.75" />
          {{ t('files.uploadFolder') }}
        </el-button>
        <el-button v-if="uploadItems.length" @click="uploadVisible = true">
          <IconListCheck :size="16" stroke="1.75" />
          {{ t('files.uploadProgress') }}
        </el-button>
        <input ref="fileInput" type="file" multiple class="hidden-input" @change="onFilesPicked">
        <input ref="folderInput" type="file" multiple webkitdirectory class="hidden-input" @change="onFilesPicked">
      </div>

      <div class="explorer">
        <aside class="tree-pane">
          <div class="pane-title">{{ t('files.folders') }}</div>
          <el-tree
            v-if="ready"
            ref="treeRef"
            lazy
            highlight-current
            node-key="path"
            :load="loadTreeNode"
            :props="{ label: 'label', isLeaf: 'leaf' }"
            @node-click="onTreeClick"
          />
        </aside>
        <section class="list-pane">
          <el-table :data="entries" stripe :empty-text="t('files.empty')" height="100%">
            <el-table-column :label="t('common.name')" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">
                <el-button v-if="row.isDir" link type="primary" class="name-btn" @click="openEntry(row)">
                  <IconFolder :size="16" stroke="1.75" />
                  {{ row.name }}
                </el-button>
                <el-button
                  v-else-if="isEditableTextFile(row.name)"
                  link
                  type="primary"
                  class="name-btn"
                  :disabled="!ready || busy || uploading || atRoots || editing || dirLoading"
                  @click="openEditor(row)"
                >
                  <IconFile :size="16" stroke="1.75" />
                  {{ row.name }}
                </el-button>
                <span v-else class="file-name">
                  <IconFile :size="16" stroke="1.75" />
                  {{ row.name }}
                </span>
              </template>
            </el-table-column>
            <el-table-column :label="t('common.size')" width="120">
              <template #default="{ row }">
                {{ row.isDir ? '-' : formatSize(row.size) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('files.modified')" width="180">
              <template #default="{ row }">
                {{ formatMtime(row.mtime) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('common.actions')" width="340" fixed="right">
              <template #default="{ row }">
                <el-button
                  v-if="!row.isDir && isEditableTextFile(row.name)"
                  size="small"
                  type="primary"
                  plain
                  :disabled="!ready || busy || uploading || atRoots || editing || dirLoading"
                  @click="openEditor(row)"
                >
                  <IconFileCode :size="16" stroke="1.75" />
                  {{ t('files.edit') }}
                </el-button>
                <el-button
                  v-if="!row.isDir"
                  size="small"
                  :disabled="!ready || busy || uploading || atRoots || editing || dirLoading"
                  @click="downloadRow(row)"
                >
                  <IconDownload :size="16" stroke="1.75" />
                  {{ t('files.download') }}
                </el-button>
                <el-button
                  size="small"
                  :disabled="!ready || busy || uploading || atRoots || editing || dirLoading"
                  @click="renameRow(row)"
                >
                  <IconPencil :size="16" stroke="1.75" />
                  {{ t('files.rename') }}
                </el-button>
                <el-button
                  size="small"
                  type="danger"
                  plain
                  :disabled="!ready || busy || uploading || atRoots || editing || dirLoading"
                  @click="removeRow(row)"
                >
                  <IconTrash :size="16" stroke="1.75" />
                  {{ t('common.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </section>
      </div>
    </div>

    <FileEditDialog
      ref="editorRef"
      v-model:active="editing"
      :asset-id="assetId"
      @saved="refresh"
    />

    <UploadTasksDialog
      v-model="uploadVisible"
      :items="uploadItems"
      :uploading="uploading"
      :overall-speed="overallSpeed"
      :now-tick="uploadTick"
      @resume="resumeFailedUploads"
      @cancel="cancelAll"
    />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  IconArrowUp,
  IconDownload,
  IconFile,
  IconFileCode,
  IconFolder,
  IconFolderUp,
  IconListCheck,
  IconPencil,
  IconRefresh,
  IconTrash,
  IconUpload,
  IconX
} from '@tabler/icons-vue'
import api from '../../shared/api'
import SessionAssetTitle from '../../session/SessionAssetTitle.vue'
import SessionConnectingMask from '../../session/SessionConnectingMask.vue'
import FileEditDialog from './FileEditDialog.vue'
import UploadTasksDialog from '../filetransfer/UploadTasksDialog.vue'
import { isEditableTextFile } from '../filetransfer/textFiles'
import { downloadToDisk } from '../filetransfer/fileDownload'
import { formatMtime, formatSize } from './fileFormat'
import { useUploadQueue } from '../filetransfer/uploadQueue'
import { closeSessionTab, rewriteWs } from '../../session/sessionWs'
import { joinPath as joinPathOs, joinRelative as joinRelativeOs, parentPath as parentPathOs, sortEntries } from './filePaths'
import { createFileRpc } from './fileRpc'

const { t } = useI18n()
const route = useRoute()
const assetId = computed(() => String(route.params.assetId || ''))
const title = ref('')
const assetInfo = ref(null)
const status = ref('connecting')
const error = ref('')
const path = ref('')
const platform = ref('')
const entries = ref([])
const busy = ref(false)
const downloadMask = ref(null) // { name, detail } while downloading
const ready = ref(false)
const dirLoading = ref(false)
let dirLoadDepth = 0
const fileInput = ref(null)
const folderInput = ref(null)
const treeRef = ref(null)

const uploadVisible = ref(false)
const {
  items: uploadItems,
  uploading,
  nowTick: uploadTick,
  overallSpeed,
  restoreStored,
  queueFiles,
  resumeFailed,
  cancelAll,
  dispose: disposeUploads
} = useUploadQueue({
  assetId,
  prepareRemotePath,
  onQueueSettled: refresh
})

const editorRef = ref(null)
const editing = ref(false)

const fileRpc = createFileRpc()
let ws
const { rpc, rejectAll: rejectPending, handleMessage: handleRpcMessage, attach: attachRpc } = fileRpc

const isWindows = computed(() => {
  const os = String(platform.value || '').toLowerCase()
  return os.includes('windows') || os === 'windows'
})
const platformLabel = computed(() => (isWindows.value ? 'Windows' : (platform.value || 'Unix')))
const atRoots = computed(() => {
  const p = String(path.value || '')
  return !p || p === 'roots' || (isWindows.value && p === '/')
})
const pathPlaceholder = computed(() => (isWindows.value ? 'C:\\' : '/'))

const statusTag = computed(() => {
  if (status.value === 'connected') return 'success'
  if (status.value === 'error') return 'danger'
  return 'info'
})

const showBusyMask = computed(() => (
  status.value === 'connecting' || dirLoading.value || busy.value
))
const busyMaskText = computed(() => {
  if (status.value === 'connecting') return t('files.connecting')
  if (dirLoading.value) return t('files.loadingDir')
  if (downloadMask.value?.name) return t('files.downloading', { name: downloadMask.value.name })
  return t('common.processing')
})
const busyMaskDetail = computed(() => {
  if (downloadMask.value?.detail) return downloadMask.value.detail
  return ''
})

function beginDirLoad() {
  dirLoadDepth += 1
  dirLoading.value = true
}

function endDirLoad() {
  dirLoadDepth = Math.max(0, dirLoadDepth - 1)
  if (dirLoadDepth === 0) dirLoading.value = false
}

const goBack = closeSessionTab

function joinPath(dir, name) {
  return joinPathOs(dir, name, isWindows.value)
}

function joinRelative(dir, relative) {
  return joinRelativeOs(dir, relative, isWindows.value)
}

function parentPath(dir) {
  return parentPathOs(dir, isWindows.value)
}

function entryPath(row) {
  if (row.path) return row.path
  return joinPath(path.value, row.name)
}

async function refresh() {
  if (!ready.value) return
  beginDirLoad()
  try {
    const listPath = atRoots.value ? (isWindows.value ? 'roots' : '/') : (path.value || '/')
    const result = await rpc('list', { path: listPath })
    if (result?.platform) platform.value = result.platform
    if (result?.path && !atRoots.value) path.value = result.path
    entries.value = sortEntries(result?.entries || [])
  } catch (e) {
    ElMessage.error(e.message || t('files.listFailed'))
  } finally {
    endDirLoad()
  }
}

async function bootstrapPath(assetOs) {
  beginDirLoad()
  try {
    if (assetOs) platform.value = assetOs
    try {
      const result = await rpc('list', { path: 'roots' })
      if (result?.platform) platform.value = result.platform
      const list = result?.entries || []
      if (isWindows.value) {
        path.value = list[0]?.path || 'C:\\'
      } else {
        path.value = '/'
      }
    } catch {
      path.value = isWindows.value ? 'C:\\' : '/'
    }
    await refresh()
  } finally {
    endDirLoad()
  }
}

function openEntry(row) {
  if (!row.isDir) return
  path.value = entryPath(row)
  refresh()
}

function goUp() {
  if (atRoots.value) return
  path.value = parentPath(path.value)
  refresh()
}

function toTreeNodes(entries, parentPath) {
  return sortEntries(entries || [])
    .filter((e) => e.isDir)
    .map((e) => ({
      label: e.name === '/' ? '/' : e.name,
      path: e.path || joinPath(parentPath || '/', e.name),
      leaf: false
    }))
}

async function loadTreeNode(node, resolve) {
  beginDirLoad()
  try {
    if (!ready.value) {
      resolve([])
      return
    }
    if (node.level === 0) {
      const result = await rpc('list', { path: 'roots' })
      if (result?.platform) platform.value = result.platform
      let nodes = toTreeNodes(result?.entries, '/')
      // Still empty: force a "/" node so the pane is never blank.
      if (!nodes.length) {
        nodes = [{ label: '/', path: '/', leaf: false }]
      }
      resolve(nodes)
      return
    }
    const result = await rpc('list', { path: node.data.path })
    resolve(toTreeNodes(result?.entries, node.data.path))
  } catch (e) {
    console.warn('loadTreeNode failed', e)
    resolve(node.level === 0 ? [{ label: '/', path: '/', leaf: false }] : [])
  } finally {
    endDirLoad()
  }
}

function onTreeClick(data) {
  if (!data?.path) return
  path.value = data.path
  refresh()
}

function pickFiles() {
  fileInput.value?.click()
}

function pickFolder() {
  folderInput.value?.click()
}

function onFilesPicked(ev) {
  const list = Array.from(ev.target.files || [])
  ev.target.value = ''
  if (!list.length) return
  if (atRoots.value) {
    ElMessage.warning(isWindows.value ? t('files.enterDriveFirst') : t('files.enterDirFirst'))
    return
  }
  uploadVisible.value = true
  queueFiles(list).catch((e) => ElMessage.error(e.message || t('files.cannotStartUpload')))
}

/** Resolve an upload's remote path against the current directory, creating its parent. */
async function prepareRemotePath(relativePath) {
  const remote = joinRelative(path.value, relativePath)
  const parent = parentPath(remote)
  if (parent && parent !== remote && parent !== 'roots') {
    await rpc('mkdir', { path: parent })
  }
  return remote
}

async function resumeFailedUploads() {
  uploadVisible.value = true
  await resumeFailed()
}

async function renameRow(row) {
  try {
    const { value } = await ElMessageBox.prompt(t('files.renamePrompt'), t('files.renameTitle'), {
      inputValue: row.name,
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      inputValidator: (v) => {
        if (!v || !String(v).trim()) return t('common.nameRequired')
        if (/[/\\]/.test(v)) return t('files.nameHasSeparator')
        return true
      }
    })
    const name = String(value).trim()
    if (name === row.name) return
    busy.value = true
    const from = entryPath(row)
    const to = joinPath(path.value, name)
    await rpc('rename', { path: from, to })
    ElMessage.success(t('common.renameSuccess'))
    await refresh()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e.message || t('common.renameFailed'))
  } finally {
    busy.value = false
  }
}

async function removeRow(row) {
  try {
    await ElMessageBox.confirm(
      row.isDir ? t('files.deleteDirConfirm', { name: row.name }) : t('files.deleteFileConfirm', { name: row.name }),
      t('files.deleteTitle'),
      { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') }
    )
    busy.value = true
    await rpc('remove', { path: entryPath(row) })
    ElMessage.success(t('common.deleteSuccess'))
    await refresh()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e.message || t('common.deleteFailed'))
  } finally {
    busy.value = false
  }
}

async function downloadRow(row) {
  if (row.isDir) return
  busy.value = true
  downloadMask.value = { name: row.name, detail: t('common.preparing') }
  try {
    await downloadToDisk({
      assetId: assetId.value,
      remotePath: entryPath(row),
      fileName: row.name,
      onProgress: (p) => {
        if (p.phase === 'pick-save') {
          downloadMask.value = { name: row.name, detail: t('files.chooseSaveLocation') }
          return
        }
        const total = Number(p.total) || 0
        const loaded = Number(p.loaded) || 0
        if (p.status === 'retrying') {
          downloadMask.value = {
            name: row.name,
            detail: total > 0
              ? t('files.resumeTransfer', { loaded: formatSize(loaded), total: formatSize(total) })
              : t('files.resumeTransferEllipsis')
          }
          return
        }
        if (p.phase === 'start' && loaded === 0) {
          downloadMask.value = {
            name: row.name,
            detail: total > 0
              ? t('files.transferProgressZero', { total: formatSize(total) })
              : t('files.transferStart')
          }
          return
        }
        const pct = total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : 0
        downloadMask.value = {
          name: row.name,
          detail: total > 0
            ? `${formatSize(loaded)} / ${formatSize(total)} (${pct}%)`
            : t('files.received', { size: formatSize(loaded) })
        }
      }
    })
  } finally {
    busy.value = false
    downloadMask.value = null
  }
}

function openEditor(row) {
  if (row.isDir) return
  editorRef.value?.open({ path: entryPath(row), name: row.name, size: row.size })
}

onMounted(async () => {
  title.value = t('files.title')
  try {
    restoreStored()
    const { data } = await api.post('/sessions/ticket', {
      assetId: route.params.assetId,
      protocol: 'filemanager'
    })
    title.value = data.asset?.displayName
      ? t('files.titleWithAsset', { name: data.asset.displayName })
      : t('files.title')
    assetInfo.value = data.asset || null

    if (data.implemented === false || !data.browserWs) {
      error.value = data.message || t('files.protocolNotReady')
      status.value = 'error'
      return
    }

    ws = new WebSocket(rewriteWs(data.browserWs))
    attachRpc(ws)
    ws.onopen = async () => {
      // Keep status=connecting (full-page mask) until the first directory list finishes.
      ready.value = true
      try {
        await bootstrapPath(data.asset?.os || '')
        status.value = 'connected'
      } catch (e) {
        status.value = 'error'
        error.value = e.message || t('files.loadDirFailed')
        ready.value = false
      }
    }
    ws.onmessage = (ev) => {
      const raw = typeof ev.data === 'string' ? ev.data : new TextDecoder().decode(ev.data)
      handleRpcMessage(raw)
    }
    ws.onclose = () => {
      if (!ready.value) error.value = t('files.wsNotEstablished')
      status.value = 'closed'
      ready.value = false
      // Otherwise save/upload awaits forever with a spinning button.
      rejectPending(t('files.sessionDisconnected'))
    }
    ws.onerror = () => {
      if (!ready.value) error.value = t('files.wsError')
      status.value = 'error'
      ready.value = false
      rejectPending(t('files.wsError'))
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.response?.data?.message || e.message || t('files.createSessionFailed')
    status.value = 'error'
  }
})

onBeforeUnmount(() => {
  disposeUploads()
  rejectPending('closed')
  ws?.close()
})
</script>

<style scoped>
.files-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 10px 12px;
  box-sizing: border-box;
  background: var(--ops-bg-page);
}
.bar {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 10px;
  padding: 8px 12px;
  background: var(--ops-bg-surface);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  flex-shrink: 0;
}
.bar :deep(.session-titles) { flex: 1; min-width: 120px; }
.files-body { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
.path-bar { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.path-bar .el-input { flex: 1; min-width: 220px; }
.bar :deep(.el-button),
.path-bar :deep(.el-button),
.list-pane :deep(.el-button),
.error-box :deep(.el-button) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.name-btn { padding: 0; height: auto; }
.file-name {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}
.error-box { flex: 1; display: flex; align-items: center; justify-content: center; }
.hidden-input { display: none; }
.explorer {
  position: relative;
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: 10px;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
}
.tree-pane {
  border-right: 1px solid #ebeef5;
  overflow: auto;
  padding: 8px;
  background: #fafafa;
}
.list-pane { min-width: 0; overflow: hidden; padding: 0 4px; }
.pane-title { font-size: 12px; color: #909399; margin-bottom: 8px; }
</style>
