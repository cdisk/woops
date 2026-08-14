export const portmapRoutes = [
  {
    path: 'portmaps',
    component: () => import('./PortMapsPage.vue'),
    meta: { feature: 'portmap', titleKey: 'portmap.pageTitle' }
  }
]
