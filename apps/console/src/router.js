import { createRouter, createWebHistory } from 'vue-router'
import Login from './modules/auth/Login.vue'
import Layout from './shared/Layout.vue'
import Assets from './modules/assets/Assets.vue'
import Users from './modules/users/Users.vue'
import { standaloneRoutes, layoutRoutes } from './features/registry'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login },
    // Standalone pages (no sidebar) — each opens in its own browser tab.
    ...standaloneRoutes,
    {
      path: '/',
      component: Layout,
      children: [
        { path: '', redirect: '/home' },
        { path: 'assets', component: Assets, meta: { titleKey: 'assets.title' } },
        { path: 'users', component: Users, meta: { roles: ['SUPER_ADMIN', 'ADMIN'], titleKey: 'users.title' } },
        ...layoutRoutes
        // Future routes: cicd
      ]
    }
  ]
})

router.beforeEach((to) => {
  if (to.path === '/login') return true
  if (!localStorage.getItem('token')) {
    return '/login'
  }
  const need = to.meta?.roles
  if (need?.length) {
    const role = localStorage.getItem('role')
    if (!need.includes(role)) return '/home'
  }
})

export default router
