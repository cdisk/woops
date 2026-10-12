export const monitorSessionRoutes = [
  {
    path: '/assets/:id/monitor',
    component: () => import('./AssetMonitorPage.vue'),
    meta: { session: true, feature: 'monitor' }
  }
]

export const monitorLayoutRoutes = [
  {
    path: 'home',
    component: () => import('./Home.vue'),
    meta: { feature: 'monitor', titleKey: 'home.title' }
  },
  {
    path: 'monitor/alerts',
    component: () => import('./AlertRules.vue'),
    meta: { roles: ['SUPER_ADMIN'], feature: 'monitor', titleKey: 'monitor.alertsTitle' }
  },
  {
    path: 'monitor/notifications',
    component: () => import('./NotifyWebhooks.vue'),
    meta: { roles: ['SUPER_ADMIN'], feature: 'monitor', titleKey: 'monitor.notifyTitle' }
  },
  {
    path: 'monitor/incidents',
    component: () => import('./AlertIncidents.vue'),
    meta: { feature: 'monitor', titleKey: 'monitor.incidentsTitle' }
  }
]
