import { shellRoutes } from './shell/routes'
import { shellActions } from './shell/actions'
import { filemanagerRoutes } from './filemanager/routes'
import { filemanagerActions } from './filemanager/actions'
import { desktopRoutes } from './desktop/routes'
import { desktopActions } from './desktop/actions'
import { monitorSessionRoutes, monitorLayoutRoutes } from './monitor/routes'
import { monitorActions } from './monitor/actions'
import { portmapRoutes } from './portmap/routes'
import { auditRoutes, auditMenu } from './audit/routes'
import { profileLayoutRoutes } from './profile/routes'

/** Standalone session/monitor pages (outside Layout). */
export const standaloneRoutes = [
  ...shellRoutes,
  ...filemanagerRoutes,
  ...desktopRoutes,
  ...monitorSessionRoutes
]

/** Feature-owned Layout child routes (portmap, monitor alerts, audit, profile). */
export const layoutRoutes = [
  ...portmapRoutes,
  ...monitorLayoutRoutes,
  ...auditRoutes,
  ...profileLayoutRoutes
]

/** Sidebar audit submenu. */
export { auditMenu }

/** Asset list session buttons (shell / desktop / files / monitor). */
export const assetSessionActions = [
  ...shellActions,
  ...desktopActions,
  ...filemanagerActions,
  ...monitorActions
]
