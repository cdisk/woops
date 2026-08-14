<template>
  <el-container class="layout">
    <el-aside :width="asideWidth" class="aside">
      <div class="brand">
        <img class="brand-mark" src="/favicon.svg" width="22" height="22" alt="" />
        <span class="brand-text">Woops</span>
      </div>
      <el-menu
        router
        :default-active="activeMenu"
        class="side-menu"
      >
        <el-menu-item index="/home">
          <IconHome class="nav-icon" :size="18" stroke-width="1.75" />
          <span>{{ t('nav.home') }}</span>
        </el-menu-item>
        <el-menu-item index="/assets">
          <IconServer class="nav-icon" :size="18" stroke-width="1.75" />
          <span>{{ t('nav.assets') }}</span>
        </el-menu-item>
        <el-menu-item index="/portmaps">
          <IconNetwork class="nav-icon" :size="18" stroke-width="1.75" />
          <span>{{ t('nav.portmaps') }}</span>
        </el-menu-item>
        <el-menu-item v-if="showAlerts" index="/monitor/alerts">
          <IconBell class="nav-icon" :size="18" stroke-width="1.75" />
          <span>{{ t('nav.alerts') }}</span>
        </el-menu-item>
        <el-menu-item v-if="showUsers" index="/users">
          <IconUsers class="nav-icon" :size="18" stroke-width="1.75" />
          <span>{{ t('nav.users') }}</span>
        </el-menu-item>
        <el-sub-menu index="/audit">
          <template #title>
            <IconClipboardList class="nav-icon" :size="18" stroke-width="1.75" />
            <span>{{ t('nav.audit') }}</span>
          </template>
          <el-menu-item
            v-for="item in auditMenuItems"
            :key="item.index"
            :index="item.index"
          >{{ item.title }}</el-menu-item>
        </el-sub-menu>
      </el-menu>
    </el-aside>
    <el-container class="main-wrap">
      <el-header v-if="!isReplay" class="header" height="var(--ops-header-height)">
        <h1 class="header-title">{{ pageTitle }}</h1>
        <div class="header-right">
          <span class="meta">{{ user }} · {{ roleLabel }}</span>
          <el-button link type="danger" class="logout" @click="logout">{{ t('nav.logout') }}</el-button>
        </div>
      </el-header>
      <el-main :class="{ 'replay-main': isReplay }">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  IconBell,
  IconClipboardList,
  IconHome,
  IconNetwork,
  IconServer,
  IconUsers
} from '@tabler/icons-vue'
import { canManageUsers, clearSession, getDisplayName, getRole, isSuperAdmin, refreshMe } from './auth'
import { auditMenu } from '../features/registry'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const role = ref(getRole())
const user = computed(() => getDisplayName() || 'user')
const showUsers = computed(() => canManageUsers())
const showAlerts = computed(() => isSuperAdmin())
const asideWidth = 'var(--ops-aside-width)'
const roleLabel = computed(() => {
  const key = `roles.${role.value}`
  const label = t(key)
  return label !== key ? label : (role.value || '-')
})

const auditMenuItems = computed(() =>
  auditMenu.map((item) => ({
    index: item.index,
    title: t(item.titleKey)
  }))
)

const isReplay = computed(() =>
  route.path.startsWith('/audit/operations/') && route.path.endsWith('/play'))

const pageTitle = computed(() => {
  const key = [...route.matched].reverse().find((r) => r.meta?.titleKey)?.meta?.titleKey
  if (!key) return ''
  const label = t(key)
  return label !== key ? label : ''
})

const activeMenu = computed(() =>
  route.path.startsWith('/audit/operations') ? '/audit/operations' : route.path)

function logout() {
  clearSession()
  router.push('/login')
}

onMounted(async () => {
  try {
    const me = await refreshMe()
    role.value = me.role
  } catch {
    // interceptor may redirect
  }
})
</script>

<style scoped>
.layout {
  height: 100%;
  background: var(--ops-bg-page);
}

.aside {
  background: var(--ops-bg-aside);
  border-right: 1px solid var(--ops-border);
  display: flex;
  flex-direction: column;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: var(--ops-header-height);
  padding: 0 18px;
  border-bottom: 1px solid var(--ops-border);
  flex-shrink: 0;
}

.brand-mark {
  width: 22px;
  height: 22px;
  border-radius: 4px;
  flex-shrink: 0;
  display: block;
}

.brand-text {
  font-weight: 600;
  font-size: 17px;
  letter-spacing: -0.03em;
  color: var(--ops-text);
}

.side-menu {
  border-right: none;
  padding: 10px 8px;
  background: transparent;
}

.side-menu :deep(.el-menu-item),
.side-menu :deep(.el-sub-menu__title) {
  height: 40px;
  line-height: 40px;
  margin: 2px 0;
  border-radius: var(--ops-radius-sm);
  color: var(--ops-text);
}

.side-menu :deep(.el-menu-item.is-active) {
  border-right: none;
  font-weight: 500;
}

.nav-icon {
  margin-right: 10px;
  color: var(--ops-text-secondary);
  flex-shrink: 0;
  vertical-align: middle;
}

.side-menu :deep(.el-menu-item.is-active) .nav-icon {
  color: var(--ops-color-primary);
}

.main-wrap {
  min-width: 0;
  min-height: 0;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 20px;
  background: var(--ops-bg-surface);
  border-bottom: 1px solid var(--ops-border);
}

.header-title {
  margin: 0;
  min-width: 0;
  flex: 1;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--ops-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.meta {
  color: var(--ops-text-secondary);
  font-size: 13px;
}

.logout {
  font-weight: 500;
}

.layout :deep(.el-main) {
  padding: 16px 20px;
  background: var(--ops-bg-page);
}

.replay-main {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  padding: 0 !important;
}
</style>
