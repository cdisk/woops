import { ElMessage } from 'element-plus'
import { t } from '../i18n'

/** Open a session/monitor page in a new browser tab (unique window name). */
export function openSessionTab(path, kind, assetId) {
  const name = `ops-${kind}-${assetId}-${Date.now()}`
  const win = window.open(path, name)
  if (!win) {
    ElMessage.warning(t('session.popupBlocked'))
  }
}
