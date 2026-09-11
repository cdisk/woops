<template>
  <div class="deploy-token-panel">
    <div class="panel-head">
      <h3>{{ t('assets.deployTokenSection') }}</h3>
      <el-button size="small" link type="primary" @click="downloadDialogVisible = true">
        {{ t('deployToken.downloadWoopsctl') }}
      </el-button>
    </div>
    <div class="toolbar">
      <el-button type="primary" size="small" @click="openCreate">{{ t('deployToken.create') }}</el-button>
      <el-button size="small" :loading="loading" @click="reload">{{ t('common.refresh') }}</el-button>
    </div>

    <el-table :data="rows" v-loading="loading" size="small" stripe :empty-text="t('deployToken.empty')">
      <el-table-column label="Token" width="100">
        <template #default="{ row }">
          <code class="token-id">{{ shortId(row.id) }}</code>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.remark')" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">{{ row.remark || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('deployToken.scopes')" min-width="320">
        <template #default="{ row }">
          <el-tag v-if="row.allowUpload" size="small" class="scope-tag">{{ t('deployToken.scopeUpload') }}</el-tag>
          <el-tag v-if="row.allowDownload" size="small" class="scope-tag">{{ t('deployToken.scopeDownload') }}</el-tag>
          <el-tag v-if="row.allowExec" size="small" type="warning" class="scope-tag">{{ t('deployToken.scopeExec') }}</el-tag>
          <el-tag v-if="row.allowForward" size="small" type="success" class="scope-tag">{{ t('deployToken.scopeForward') }}</el-tag>
          <el-tag v-if="row.allowReverse" size="small" type="success" class="scope-tag">{{ t('deployToken.scopeReverse') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('deployToken.expires')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.expiresAt ? formatTime(row.expiresAt) : t('deployToken.never') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.status')" width="90" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="row.active ? 'success' : 'info'">
            {{ row.active ? t('deployToken.active') : (row.revokedAt ? t('deployToken.revoked') : t('deployToken.expired')) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('deployToken.createdAt')" min-width="150" show-overflow-tooltip>
        <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('deployToken.lastUsed')" min-width="150" show-overflow-tooltip>
        <template #default="{ row }">{{ row.lastUsedAt ? formatTime(row.lastUsedAt) : t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="90" fixed="right" align="center">
        <template #default="{ row }">
          <el-button
            size="small"
            link
            type="danger"
            :disabled="!!row.revokedAt"
            @click="revoke(row)"
          >{{ t('deployToken.revoke') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="createVisible" :title="t('deployToken.createTitle')" width="640px" destroy-on-close>
      <el-form label-width="96px">
        <el-form-item :label="t('common.remark')">
          <el-input
            v-model="form.remark"
            type="textarea"
            :rows="2"
            maxlength="512"
            show-word-limit
            :placeholder="t('deployToken.remarkPlaceholder')"
          />
        </el-form-item>
        <el-form-item class="scopes-field" :label="t('deployToken.scopes')" required>
          <div class="scope-list">
            <div class="scope-item">
              <el-checkbox v-model="form.allowUpload">{{ t('deployToken.scopeUpload') }}</el-checkbox>
              <span class="scope-desc">{{ t('deployToken.scopeUploadDesc') }}</span>
            </div>
            <div class="scope-item">
              <el-checkbox v-model="form.allowDownload">{{ t('deployToken.scopeDownload') }}</el-checkbox>
              <span class="scope-desc">{{ t('deployToken.scopeDownloadDesc') }}</span>
            </div>
            <div class="scope-item">
              <el-checkbox v-model="form.allowExec">{{ t('deployToken.scopeExec') }}</el-checkbox>
              <span class="scope-desc">{{ t('deployToken.scopeExecDesc') }}</span>
            </div>
            <div class="scope-item">
              <el-checkbox v-model="form.allowForward">{{ t('deployToken.scopeForward') }}</el-checkbox>
              <span class="scope-desc">{{ t('deployToken.scopeForwardDesc') }}</span>
            </div>
            <div class="scope-item">
              <el-checkbox v-model="form.allowReverse">{{ t('deployToken.scopeReverse') }}</el-checkbox>
              <span class="scope-desc">{{ t('deployToken.scopeReverseDesc') }}</span>
            </div>
          </div>
        </el-form-item>
        <el-form-item :label="t('deployToken.expires')">
          <el-date-picker
            v-model="form.expiresAt"
            type="datetime"
            :placeholder="t('deployToken.expiresPlaceholder')"
            clearable
            style="width: 100%"
          />
        </el-form-item>
        <p class="form-tip">{{ t('deployToken.formTip') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="create">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="secretVisible" :title="t('deployToken.secretTitle')" width="640px" :close-on-click-modal="false">
      <p class="warn">{{ t('deployToken.secretWarn') }}</p>
      <el-input type="textarea" :rows="8" :model-value="createdConfigJson" readonly />
      <el-button size="small" class="copy-btn" @click="copyConfig">
        {{ t('deployToken.copyConfig') }}
      </el-button>
    </el-dialog>

    <el-dialog
      v-model="downloadDialogVisible"
      :title="t('deployToken.downloadWoopsctl')"
      width="720px"
      destroy-on-close
    >
      <p class="download-tip">{{ t('deployToken.downloadTip') }}</p>
      <div
        v-for="item in woopsctlDownloads"
        :key="item.key"
        class="download-row"
      >
        <div class="download-meta">
          <span class="download-label">{{ item.label }}</span>
          <code class="download-url">{{ item.url }}</code>
        </div>
        <div class="download-actions">
          <el-button size="small" @click="copyWoopsctlUrl(item)">{{ t('deployToken.copyDownloadUrl') }}</el-button>
          <el-button size="small" @click="copyWoopsctlWget(item)">{{ t('deployToken.copyWget') }}</el-button>
          <el-button
            size="small"
            type="primary"
            :loading="downloadingKey === item.key"
            :disabled="!!downloadingKey && downloadingKey !== item.key"
            @click="downloadWoopsctl(item)"
          >{{ t('deployToken.downloadFile') }}</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { formatTime } from '../../shared/format'

const props = defineProps({
  assetId: { type: String, required: true }
})

const { t } = useI18n()

const rows = ref([])
const loading = ref(false)
const createVisible = ref(false)
const creating = ref(false)
const secretVisible = ref(false)
const createdConfigJson = ref('')
const downloadDialogVisible = ref(false)
const downloadingKey = ref('')

function woopsctlPublicUrl(os, arch) {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  return `${origin}/bin/woopsctl/${os}/${arch}`
}

const woopsctlDownloads = computed(() => [
  {
    key: 'linux-amd64',
    os: 'linux',
    arch: 'amd64',
    label: t('deployToken.downloadLinuxAmd64'),
    filename: 'woopsctl',
    url: woopsctlPublicUrl('linux', 'amd64')
  },
  {
    key: 'linux-arm64',
    os: 'linux',
    arch: 'arm64',
    label: t('deployToken.downloadLinuxArm64'),
    filename: 'woopsctl',
    url: woopsctlPublicUrl('linux', 'arm64')
  },
  {
    key: 'windows-amd64',
    os: 'windows',
    arch: 'amd64',
    label: t('deployToken.downloadWindowsAmd64'),
    filename: 'woopsctl.exe',
    url: woopsctlPublicUrl('windows', 'amd64')
  }
])

const form = reactive({
  remark: '',
  allowUpload: true,
  allowDownload: false,
  allowExec: true,
  allowForward: false,
  allowReverse: false,
  expiresAt: null
})

async function copyText(text, okMessage) {
  await navigator.clipboard.writeText(text)
  ElMessage.success(okMessage)
}

async function copyWoopsctlUrl(item) {
  await copyText(item.url, t('deployToken.copiedDownloadUrl'))
}

async function copyWoopsctlWget(item) {
  const cmd = item.os === 'windows'
    ? `curl -fsSL "${item.url}" -o ${item.filename}`
    : `wget -qO ${item.filename} "${item.url}" && chmod +x ${item.filename}`
  await copyText(cmd, t('deployToken.copiedWget'))
}

async function downloadWoopsctl(item) {
  if (downloadingKey.value) return
  downloadingKey.value = item.key
  try {
    // fetch + blob avoids Chrome's flaky "network error" on synthetic <a download> for multi-MB files
    const res = await fetch(`/bin/woopsctl/${item.os}/${item.arch}`)
    if (!res.ok) {
      throw new Error(t('deployToken.downloadHttpFailed', { status: res.status }))
    }
    const blob = await res.blob()
    const objUrl = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = objUrl
    a.download = item.filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(objUrl)
  } catch (e) {
    ElMessage.error(e?.message || t('deployToken.downloadFailed'))
  } finally {
    downloadingKey.value = ''
  }
}

async function reload() {
  if (!props.assetId) return
  loading.value = true
  try {
    const { data } = await api.get(`/assets/${props.assetId}/deploy-tokens`)
    rows.value = Array.isArray(data) ? data : []
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.remark = ''
  form.allowUpload = true
  form.allowDownload = false
  form.allowExec = true
  form.allowForward = false
  form.allowReverse = false
  form.expiresAt = null
  createVisible.value = true
}

async function create() {
  if (!form.allowUpload && !form.allowDownload && !form.allowExec && !form.allowForward && !form.allowReverse) {
    ElMessage.warning(t('deployToken.scopeRequired'))
    return
  }
  creating.value = true
  try {
    let expiresAt = null
    if (form.expiresAt) {
      const d = form.expiresAt instanceof Date ? form.expiresAt : new Date(form.expiresAt)
      if (Number.isNaN(d.getTime())) {
        ElMessage.warning(t('deployToken.expiresInvalid'))
        return
      }
      expiresAt = d.toISOString()
    }
    const { data } = await api.post(`/assets/${props.assetId}/deploy-tokens`, {
      remark: form.remark.trim() || null,
      allowUpload: form.allowUpload,
      allowDownload: form.allowDownload,
      allowExec: form.allowExec,
      allowForward: form.allowForward,
      allowReverse: form.allowReverse,
      expiresAt
    })
    createVisible.value = false
    createdConfigJson.value = data.opsctlConfigJson
      || (data.opsctlConfig ? JSON.stringify(data.opsctlConfig, null, 2) : '')
    if (!createdConfigJson.value) {
      ElMessage.error(t('deployToken.noConfigReturned'))
      return
    }
    secretVisible.value = true
    await reload()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('common.createFailed'))
  } finally {
    creating.value = false
  }
}

async function revoke(row) {
  const label = row.remark || row.id?.slice?.(0, 8) || t('deployToken.thisToken')
  try {
    await ElMessageBox.confirm(
      t('deployToken.revokeConfirm', { label }),
      t('deployToken.revokeTitle'),
      { confirmButtonText: t('deployToken.revoke'), cancelButtonText: t('common.cancel'), type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }
  try {
    await api.delete(`/assets/${props.assetId}/deploy-tokens/${row.id}`)
    ElMessage.success(t('deployToken.revokedOk'))
    await reload()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('deployToken.revokeFailed'))
  }
}

async function copyConfig() {
  await navigator.clipboard.writeText(createdConfigJson.value)
  ElMessage.success(t('deployToken.copiedConfig'))
}

function shortId(id) {
  if (!id) return t('common.emDash')
  return String(id).slice(0, 8)
}

onMounted(reload)
defineExpose({ reload })
</script>

<style scoped>
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.panel-head h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; align-items: center; }
.token-id { font-size: 12px; color: #374151; }
.scope-tag { margin-right: 4px; }
.scope-list { display: flex; flex-direction: column; gap: 8px; width: 100%; padding-top: 4px; }
.scope-item { display: flex; align-items: center; gap: 10px; min-width: 0; }
.scope-item :deep(.el-checkbox) { flex: 0 0 auto; margin-right: 0; }
.scope-desc { color: #6b7280; font-size: 12px; line-height: 1.4; flex: 1; min-width: 0; }
.scopes-field :deep(.el-form-item__label) { height: auto; line-height: 32px; align-items: flex-start; }
.scopes-field :deep(.el-form-item__content) { align-items: flex-start; }
.form-tip { color: #6b7280; font-size: 12px; margin: 0 0 0 96px; }
.warn { color: #b45309; font-size: 13px; margin: 0 0 12px; }
.copy-btn { margin-top: 8px; }
.download-tip {
  margin: 0 0 16px;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.5;
}
.download-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 0;
  border-top: 1px solid #e5e7eb;
}
.download-row:last-child {
  padding-bottom: 0;
}
.download-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.download-label {
  font-size: 13px;
  font-weight: 600;
  color: #111827;
}
.download-url {
  font-size: 12px;
  color: #374151;
  word-break: break-all;
  line-height: 1.4;
}
.download-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
