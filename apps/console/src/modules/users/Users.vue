<template>
  <div class="page">
    <div class="page-toolbar">
      <el-input
        v-model="searchQuery"
        clearable
        class="toolbar-search"
        :placeholder="t('users.searchPlaceholder')"
      />
      <el-button v-if="canCreate" type="primary" @click="openCreate">{{ t('users.create') }}</el-button>
    </div>
    <el-table :data="filteredUsers" v-loading="loading" stripe>
      <el-table-column prop="username" :label="t('users.username')" min-width="120" />
      <el-table-column prop="nickname" :label="t('users.nickname')" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.nickname || t('common.emDash') }}</template>
      </el-table-column>
      <el-table-column prop="role" :label="t('users.role')" width="140">
        <template #default="{ row }">{{ roleLabel(row.role) }}</template>
      </el-table-column>
      <el-table-column prop="authSource" :label="t('users.source')" width="100" />
      <el-table-column :label="t('users.totp')" width="90">
        <template #default="{ row }">
          <template v-if="row.authSource === 'local'">
            <el-tag :type="row.totpEnabled ? 'success' : 'info'" size="small">
              {{ row.totpEnabled ? t('users.totpOn') : t('users.totpOff') }}
            </el-tag>
          </template>
          <span v-else>{{ t('common.emDash') }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('users.enabled')" width="90">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? t('common.yes') : t('common.no') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('users.scopes')" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          {{ scopeSummary(row) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="260" align="right" fixed="right">
        <template #default="{ row }">
          <el-button v-if="canEdit(row)" link type="primary" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
          <el-button v-if="canEdit(row) && row.role !== 'SUPER_ADMIN'" link type="primary" @click="openScopes(row)">{{ t('users.scopes') }}</el-button>
          <el-button v-if="canEdit(row) && row.authSource === 'local' && row.totpEnabled" link type="warning" @click="resetTotp(row)">{{ t('users.resetTotp') }}</el-button>
          <el-button v-if="canEdit(row) && row.username !== selfUsername" link type="danger" @click="removeUser(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="createVisible" :title="t('users.createTitle')" width="440px">
      <el-form label-width="90px">
        <el-form-item :label="t('users.username')">
          <el-input v-model="createForm.username" />
        </el-form-item>
        <el-form-item :label="t('users.nickname')">
          <el-input v-model="createForm.nickname" :placeholder="t('users.nicknamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('users.password')">
          <el-input v-model="createForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item v-if="isSuperAdmin()" :label="t('users.role')">
          <el-select v-model="createForm.role" style="width:100%">
            <el-option :label="t('roles.MEMBER')" value="MEMBER" />
            <el-option :label="t('roles.ADMIN')" value="ADMIN" />
            <el-option :label="t('roles.SUPER_ADMIN')" value="SUPER_ADMIN" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="createUser">{{ t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" :title="t('users.editTitle')" width="440px">
      <el-form label-width="90px">
        <el-form-item :label="t('users.username')">
          <el-input :model-value="editForm.username" disabled />
        </el-form-item>
        <el-form-item :label="t('users.nickname')">
          <el-input v-model="editForm.nickname" :placeholder="t('users.nicknamePlaceholder')" />
        </el-form-item>
        <el-form-item v-if="isSuperAdmin()" :label="t('users.role')">
          <el-select v-model="editForm.role" style="width:100%">
            <el-option :label="t('roles.MEMBER')" value="MEMBER" />
            <el-option :label="t('roles.ADMIN')" value="ADMIN" />
            <el-option :label="t('roles.SUPER_ADMIN')" value="SUPER_ADMIN" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('users.enabled')">
          <el-switch v-model="editForm.enabled" />
        </el-form-item>
        <el-form-item v-if="editForm.authSource === 'local'" :label="t('users.newPassword')">
          <el-input v-model="editForm.password" type="password" show-password :placeholder="t('users.passwordKeepPlaceholder')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="scopeVisible" :title="t('users.scopesTitle')" width="560px">
      <p class="hint">{{ t('users.scopesHint') }}</p>
      <el-tree
        ref="scopeTreeRef"
        :data="scopeTreeData"
        node-key="key"
        show-checkbox
        default-expand-all
        :props="{ label: 'label', children: 'children' }"
      />
      <template #footer>
        <el-button @click="scopeVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveScopes">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../shared/api'
import { isSuperAdmin, canManageUsers, getUsername } from '../../shared/auth'

const { t } = useI18n()

const users = ref([])
const searchQuery = ref('')
const loading = ref(false)
const saving = ref(false)
const groupTree = ref([])
const allAssets = ref([])
const scopeTreeData = ref([])
const createVisible = ref(false)
const editVisible = ref(false)
const scopeVisible = ref(false)
const scopeTreeRef = ref(null)
const scopeUserId = ref('')
const selfUsername = getUsername()
const createForm = reactive({ username: '', nickname: '', password: '', role: 'MEMBER' })
const editForm = reactive({
  id: '',
  username: '',
  nickname: '',
  role: 'MEMBER',
  enabled: true,
  password: '',
  authSource: 'local'
})

const canCreate = computed(() => canManageUsers())

const filteredUsers = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return users.value
  return users.value.filter((u) =>
    [u.username, u.nickname, u.authSource, roleLabel(u.role)]
      .some((v) => String(v || '').toLowerCase().includes(q))
  )
})

function roleLabel(r) {
  const key = `roles.${r}`
  const label = t(key)
  return label === key ? r : label
}

function canEdit(row) {
  if (!canManageUsers()) return false
  if (isSuperAdmin()) return true
  return row.role === 'MEMBER'
}

function scopeSummary(row) {
  if (row.role === 'SUPER_ADMIN') return t('users.scopeAll')
  const scopes = row.scopes || []
  if (!scopes.length) return t('users.scopeNone')
  const g = scopes.filter((s) => s.type === 'GROUP').length
  const a = scopes.filter((s) => s.type === 'ASSET').length
  const parts = []
  if (g) parts.push(t('users.scopeGroups', { n: g }))
  if (a) parts.push(t('users.scopeAssets', { n: a }))
  return parts.join(' · ') || t('users.scopeNone')
}

function buildScopeTree(groups, assets) {
  const byGroup = new Map()
  for (const a of assets || []) {
    const gid = a.groupId || '__root__'
    if (!byGroup.has(gid)) byGroup.set(gid, [])
    byGroup.get(gid).push(a)
  }
  function mapGroup(n) {
    const assetKids = (byGroup.get(n.id) || []).map((a) => ({
      key: `ASSET:${a.id}`,
      kind: 'asset',
      assetId: a.id,
      label: a.displayName || a.hostname || a.id
    }))
    const childGroups = (n.children || []).map(mapGroup)
    return {
      key: `GROUP:${n.id}`,
      kind: 'group',
      groupId: n.id,
      label: n.name,
      children: [...childGroups, ...assetKids]
    }
  }
  const roots = (groups || []).map(mapGroup)
  const ungrouped = (byGroup.get('__root__') || []).map((a) => ({
    key: `ASSET:${a.id}`,
    kind: 'asset',
    assetId: a.id,
    label: a.displayName || a.hostname || a.id
  }))
  if (ungrouped.length) {
    roots.push({
      key: 'GROUP:__ungrouped__',
      kind: 'ungrouped',
      label: t('common.ungrouped'),
      disabled: true,
      children: ungrouped
    })
  }
  return roots
}

function collectScopesFromTree() {
  const tree = scopeTreeRef.value
  if (!tree) return []
  const checked = tree.getCheckedNodes(false, false) || []
  const groupIds = new Set()
  const scopes = []
  for (const n of checked) {
    if (n.kind === 'group' && n.groupId) {
      groupIds.add(n.groupId)
      scopes.push({ type: 'GROUP', id: n.groupId })
    }
  }
  for (const n of checked) {
    if (n.kind !== 'asset' || !n.assetId) continue
    // Skip assets already covered by a checked group ancestor — server also normalizes.
    // Client: if any GROUP key is checked that contains this asset, skip.
    // Heuristic: parent walk via tree node not available; rely on: if asset's groupId is in groupIds or descendant.
    // We only have asset.groupId on asset records:
    const asset = allAssets.value.find((a) => a.id === n.assetId)
    const gid = asset?.groupId
    if (gid && isGroupCovered(gid, groupIds, groupTree.value)) continue
    scopes.push({ type: 'ASSET', id: n.assetId })
  }
  return scopes
}

function isGroupCovered(gid, selectedGroups, nodes) {
  if (selectedGroups.has(gid)) return true
  // any selected group that is ancestor of gid
  const parentOf = new Map()
  function walk(list, parent) {
    for (const n of list || []) {
      parentOf.set(n.id, parent)
      walk(n.children, n.id)
    }
  }
  walk(nodes, null)
  let cur = gid
  const seen = new Set()
  while (cur && !seen.has(cur)) {
    seen.add(cur)
    if (selectedGroups.has(cur)) return true
    cur = parentOf.get(cur)
  }
  // also: selected group is ancestor — need descendants of selected
  // Build descendants of each selected
  function descendants(id, list) {
    for (const n of list || []) {
      if (n.id === id) {
        const out = new Set([id])
        function add(ch) {
          for (const c of ch || []) {
            out.add(c.id)
            add(c.children)
          }
        }
        add(n.children)
        return out
      }
      const d = descendants(id, n.children)
      if (d) return d
    }
    return null
  }
  for (const sg of selectedGroups) {
    const d = descendants(sg, nodes)
    if (d && d.has(gid)) return true
  }
  return false
}

async function load() {
  loading.value = true
  try {
    const [{ data: u }, { data: g }, { data: a }] = await Promise.all([
      api.get('/users'),
      api.get('/groups'),
      api.get('/assets')
    ])
    users.value = u
    groupTree.value = g
    allAssets.value = a
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createForm.username = ''
  createForm.nickname = ''
  createForm.password = ''
  createForm.role = 'MEMBER'
  createVisible.value = true
}

async function createUser() {
  saving.value = true
  try {
    const body = {
      username: createForm.username,
      nickname: createForm.nickname,
      password: createForm.password,
      role: isSuperAdmin() ? createForm.role : 'MEMBER'
    }
    await api.post('/users', body)
    createVisible.value = false
    ElMessage.success(t('common.createSuccess'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.createFailed'))
  } finally {
    saving.value = false
  }
}

function openEdit(row) {
  editForm.id = row.id
  editForm.username = row.username
  editForm.nickname = row.nickname || ''
  editForm.role = row.role
  editForm.enabled = row.enabled
  editForm.password = ''
  editForm.authSource = row.authSource
  editVisible.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    const body = { enabled: editForm.enabled, nickname: editForm.nickname }
    if (isSuperAdmin()) body.role = editForm.role
    if (editForm.password) body.password = editForm.password
    await api.patch(`/users/${editForm.id}`, body)
    editVisible.value = false
    ElMessage.success(t('common.saveSuccess'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function openScopes(row) {
  scopeUserId.value = row.id
  scopeTreeData.value = buildScopeTree(groupTree.value, allAssets.value)
  scopeVisible.value = true
  await nextTick()
  const keys = (row.scopes || []).map((s) => `${s.type}:${s.id}`)
  scopeTreeRef.value?.setCheckedKeys(keys)
}

async function saveScopes() {
  saving.value = true
  try {
    const scopes = collectScopesFromTree()
    await api.put(`/users/${scopeUserId.value}/scopes`, { scopes })
    scopeVisible.value = false
    ElMessage.success(t('users.scopesUpdated'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function resetTotp(row) {
  try {
    await ElMessageBox.confirm(
      t('users.resetTotpConfirm', { name: row.nickname || row.username }),
      t('users.resetTotpTitle'),
      { type: 'warning', confirmButtonText: t('users.resetTotp'), cancelButtonText: t('common.cancel') }
    )
  } catch {
    return
  }
  saving.value = true
  try {
    await api.post(`/users/${row.id}/totp/reset`)
    ElMessage.success(t('users.resetTotpDone'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function removeUser(row) {
  try {
    await ElMessageBox.confirm(
      t('users.deleteConfirm', { name: row.nickname || row.username }),
      t('users.deleteTitle'),
      { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') }
    )
  } catch {
    return
  }
  saving.value = true
  try {
    await api.delete(`/users/${row.id}`)
    ElMessage.success(t('common.deleteSuccess'))
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('common.deleteFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.page-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.toolbar-search {
  flex: 1;
  max-width: 360px;
  min-width: 180px;
}
</style>
