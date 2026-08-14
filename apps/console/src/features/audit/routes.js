export const CONTROL_AUDIT_PATH = '/audit/control'
export const OPERATIONS_AUDIT_PATH = '/audit/operations'
export const ASSET_EVENTS_AUDIT_PATH = '/audit/asset-events'

/** Sidebar submenu under 审计 — Layout reads titleKey via t(). */
export const auditMenu = [
  { index: CONTROL_AUDIT_PATH, titleKey: 'audit.menuControl' },
  { index: OPERATIONS_AUDIT_PATH, titleKey: 'audit.menuOperations' },
  { index: ASSET_EVENTS_AUDIT_PATH, titleKey: 'audit.menuAssetEvents' }
]

export const auditRoutes = [
  { path: 'audit', redirect: CONTROL_AUDIT_PATH },
  {
    path: 'audit/control',
    component: () => import('./control/ControlAuditPage.vue'),
    meta: { feature: 'audit', titleKey: 'audit.control.title' }
  },
  {
    path: 'audit/operations',
    component: () => import('./operations/OperationsPage.vue'),
    meta: { feature: 'audit', titleKey: 'audit.operations.title' }
  },
  {
    path: 'audit/operations/:id',
    component: () => import('./operations/OperationDetailPage.vue'),
    meta: { feature: 'audit', titleKey: 'audit.operations.title' }
  },
  {
    // The asciinema player pulls in a WASM terminal emulator; keep it out of
    // the main bundle.
    path: 'audit/operations/:id/play',
    component: () => import('./operations/OperationPlayPage.vue'),
    meta: { feature: 'audit', titleKey: 'audit.play.title' }
  },
  {
    path: 'audit/asset-events',
    component: () => import('./assetevents/AssetEventsPage.vue'),
    meta: { feature: 'audit', titleKey: 'audit.assetEvents.title' }
  }
]
