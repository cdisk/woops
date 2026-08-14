export const shellRoutes = [
  {
    path: '/sessions/:assetId/shell',
    component: () => import('./ShellPage.vue'),
    meta: { session: true, feature: 'shell' }
  }
]
