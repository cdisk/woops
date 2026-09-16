<template>
  <div class="profile-page">
    <el-tabs v-model="tab">
      <el-tab-pane v-if="isLocal" :label="t('profile.account')" name="account">
        <div class="section">
          <h3 class="section-title">{{ t('profile.changePassword') }}</h3>
          <el-form class="account-form" label-position="top" @submit.prevent>
            <el-form-item :label="t('profile.currentPassword')" required>
              <el-input v-model="pwdForm.current" type="password" show-password autocomplete="current-password" />
            </el-form-item>
            <el-form-item :label="t('profile.newPassword')" required>
              <el-input v-model="pwdForm.next" type="password" show-password autocomplete="new-password" />
            </el-form-item>
            <el-form-item :label="t('profile.confirmPassword')" required>
              <el-input v-model="pwdForm.confirm" type="password" show-password autocomplete="new-password" />
            </el-form-item>
            <el-button type="primary" size="small" :loading="pwdSaving" @click="changePassword">
              {{ t('profile.savePassword') }}
            </el-button>
          </el-form>
        </div>

        <div class="section">
          <h3 class="section-title">{{ t('profile.totpTitle') }}</h3>
          <p class="hint">{{ totpEnabled ? t('profile.totpOnHint') : t('profile.totpOffHint') }}</p>
          <el-button
            v-if="totpEnabled"
            type="warning"
            size="small"
            plain
            @click="openResetTotp"
          >{{ t('profile.resetTotp') }}</el-button>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('profile.apiTokens')" name="tokens">
        <div class="toolbar">
          <p class="hint">
            {{ t('profile.apiTokensHint') }}
            <el-button link type="primary" size="small" @click="tab = 'docs'">{{ t('profile.apiDocLink') }}</el-button>
          </p>
          <el-button type="primary" size="small" @click="openCreate">{{ t('profile.createToken') }}</el-button>
        </div>

        <el-table :data="rows" v-loading="loading" stripe size="small">
          <el-table-column prop="name" :label="t('common.name')" min-width="140" />
          <el-table-column :label="t('profile.scopes')" min-width="160">
            <template #default="{ row }">
              <el-tag v-for="s in row.scopes || []" :key="s" size="small" class="scope-tag">{{ s }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('profile.expiresAt')" min-width="170">
            <template #default="{ row }">
              <span :class="{ expired: row.expired }">{{ row.expiresAt ? formatTs(row.expiresAt) : t('profile.neverExpires') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('profile.createdAt')" min-width="170">
            <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column :label="t('profile.lastUsedAt')" min-width="170">
            <template #default="{ row }">{{ row.lastUsedAt ? formatTs(row.lastUsedAt) : '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="90" fixed="right">
            <template #default="{ row }">
              <el-button link type="danger" size="small" @click="remove(row)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('profile.apiDoc')" name="docs">
        <ApiDocPanel />
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="createOpen" :title="t('profile.createToken')" width="520px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item :label="t('common.name')" required>
          <el-input v-model="form.name" maxlength="128" :placeholder="t('profile.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('profile.scopes')" required>
          <el-checkbox-group v-model="form.scopes" class="scope-checks">
            <div class="scope-row">
              <el-checkbox value="assets:read">assets:read</el-checkbox>
              <div class="scope-desc">{{ t('profile.scopeAssetsHint') }}</div>
            </div>
            <div class="scope-row">
              <el-checkbox value="metrics:read">metrics:read</el-checkbox>
              <div class="scope-desc">{{ t('profile.scopeMetricsHint') }}</div>
            </div>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="t('profile.expiresAt')">
          <el-date-picker
            v-model="form.expiresAt"
            type="datetime"
            :placeholder="t('profile.expiresDefault')"
            style="width: 100%"
          />
          <div class="field-hint">{{ t('profile.expiresHint') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button size="small" @click="createOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" size="small" :loading="creating" @click="create">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="secretOpen" :title="t('profile.tokenCreated')" width="560px" :close-on-click-modal="false">
      <el-alert type="warning" :closable="false" show-icon :title="t('profile.tokenOnce')" />
      <div class="secret-box">
        <code>{{ createdToken }}</code>
      </div>
      <template #footer>
        <el-button type="primary" size="small" @click="copySecret">{{ t('common.copy') }}</el-button>
        <el-button size="small" @click="secretOpen = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="totpOpen" :title="t('profile.resetTotp')" width="440px" destroy-on-close>
      <p class="hint">{{ t('profile.resetTotpHint') }}</p>
      <el-form label-position="top">
        <el-form-item :label="t('profile.currentPassword')" required>
          <el-input v-model="totpForm.password" type="password" show-password autocomplete="current-password" />
        </el-form-item>
        <el-form-item :label="t('login.totpCode')" required>
          <el-input v-model="totpForm.code" maxlength="6" autocomplete="one-time-code" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button size="small" @click="totpOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="warning" size="small" :loading="totpSaving" @click="resetTotp">{{ t('profile.resetTotp') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { refreshMe } from '../../shared/auth'
import ApiDocPanel from './ApiDocPanel.vue'

const { t } = useI18n()
const tab = ref('tokens')
const loading = ref(false)
const rows = ref([])
const createOpen = ref(false)
const creating = ref(false)
const secretOpen = ref(false)
const createdToken = ref('')
const authSource = ref('')
const totpEnabled = ref(false)
const pwdSaving = ref(false)
const totpOpen = ref(false)
const totpSaving = ref(false)
const form = reactive({
  name: '',
  scopes: ['assets:read', 'metrics:read'],
  expiresAt: null
})
const pwdForm = reactive({ current: '', next: '', confirm: '' })
const totpForm = reactive({ password: '', code: '' })

const isLocal = computed(() => authSource.value === 'local')

function formatTs(s) {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

async function loadMe() {
  try {
    const me = await refreshMe()
    authSource.value = me.authSource || 'local'
    totpEnabled.value = !!me.totpEnabled
    if (isLocal.value) tab.value = 'account'
    else tab.value = 'tokens'
  } catch {
    authSource.value = ''
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await api.get('/profile/api-tokens')
    rows.value = data || []
  } catch {
    ElMessage.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.name = ''
  form.scopes = ['assets:read', 'metrics:read']
  form.expiresAt = null
  createOpen.value = true
}

async function create() {
  if (!form.name.trim()) {
    ElMessage.warning(t('common.nameRequired'))
    return
  }
  if (!form.scopes.length) {
    ElMessage.warning(t('profile.scopesRequired'))
    return
  }
  creating.value = true
  try {
    const body = {
      name: form.name.trim(),
      scopes: form.scopes
    }
    if (form.expiresAt) {
      const d = form.expiresAt instanceof Date ? form.expiresAt : new Date(form.expiresAt)
      if (Number.isNaN(d.getTime())) {
        ElMessage.warning(t('profile.expiresHint'))
        return
      }
      body.expiresAt = d.toISOString()
    }
    const { data } = await api.post('/profile/api-tokens', body)
    createdToken.value = data.token || ''
    createOpen.value = false
    secretOpen.value = true
    await load()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || t('common.createFailed'))
  } finally {
    creating.value = false
  }
}

async function copySecret() {
  try {
    await navigator.clipboard.writeText(createdToken.value)
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.error(t('common.copyFailed'))
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(t('profile.deleteConfirm', { name: row.name }), t('common.delete'), {
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await api.delete(`/profile/api-tokens/${row.id}`)
    ElMessage.success(t('common.deleteSuccess'))
    await load()
  } catch {
    ElMessage.error(t('common.deleteFailed'))
  }
}

async function changePassword() {
  if (!pwdForm.current || !pwdForm.next) {
    ElMessage.warning(t('profile.passwordRequired'))
    return
  }
  if (pwdForm.next.length < 8) {
    ElMessage.warning(t('profile.passwordTooShort'))
    return
  }
  if (pwdForm.next !== pwdForm.confirm) {
    ElMessage.warning(t('profile.passwordMismatch'))
    return
  }
  pwdSaving.value = true
  try {
    await api.post('/profile/password', {
      currentPassword: pwdForm.current,
      newPassword: pwdForm.next
    })
    ElMessage.success(t('profile.passwordChanged'))
    pwdForm.current = ''
    pwdForm.next = ''
    pwdForm.confirm = ''
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || t('common.saveFailed'))
  } finally {
    pwdSaving.value = false
  }
}

function openResetTotp() {
  totpForm.password = ''
  totpForm.code = ''
  totpOpen.value = true
}

async function resetTotp() {
  if (!totpForm.password || !totpForm.code) {
    ElMessage.warning(t('profile.totpResetRequired'))
    return
  }
  totpSaving.value = true
  try {
    await api.post('/profile/totp/reset', {
      currentPassword: totpForm.password,
      totpCode: totpForm.code
    })
    totpOpen.value = false
    totpEnabled.value = false
    ElMessage.success(t('profile.totpResetDone'))
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || t('common.saveFailed'))
  } finally {
    totpSaving.value = false
  }
}

onMounted(async () => {
  await loadMe()
  await load()
})
</script>

<style scoped>
.profile-page {
  width: 100%;
  min-width: 0;
}
.toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}
.hint {
  margin: 0 0 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
  flex: 1;
}
.section {
  max-width: 420px;
  margin-bottom: 28px;
}
.section-title {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--ops-text);
}
.account-form { margin-bottom: 4px; }
.scope-tag { margin-right: 4px; }
.scope-checks {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 12px;
  width: 100%;
}
.scope-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}
.scope-desc {
  margin-left: 22px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.4;
}
.expired { color: var(--el-color-danger); }
.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.4;
}
.secret-box {
  margin-top: 12px;
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  word-break: break-all;
  font-family: 'IBM Plex Mono', ui-monospace, monospace;
  font-size: 13px;
}
</style>
