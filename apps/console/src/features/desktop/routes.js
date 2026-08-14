export const desktopRoutes = [
  {
    path: '/sessions/:assetId/rdp',
    component: () => import('./DesktopPage.vue'),
    meta: { session: true, protocol: 'rdp', feature: 'desktop' }
  },
  {
    path: '/sessions/:assetId/vnc',
    component: () => import('./DesktopPage.vue'),
    meta: { session: true, protocol: 'vnc', feature: 'desktop' }
  }
]
