export const filemanagerRoutes = [
  {
    path: '/sessions/:assetId/files',
    component: () => import('./FileManagerPage.vue'),
    meta: { session: true, feature: 'filemanager' }
  }
]
