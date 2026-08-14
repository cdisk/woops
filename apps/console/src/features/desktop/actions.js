import { IconDeviceDesktop } from '@tabler/icons-vue'
import { ElMessageBox } from 'element-plus'
import { t } from '../../i18n'
import { isWindows } from '../../session/assetOs'
import { openSessionTab } from '../../session/openSessionTab'

export const desktopActions = [
  {
    id: 'desktop',
    type: 'primary',
    icon: IconDeviceDesktop,
    requiresOnline: true,
    match: () => true,
    title: (asset) => (isWindows(asset) ? 'RDP' : 'VNC'),
    open: async (asset, { openDetail } = {}) => {
      const protocol = isWindows(asset) ? 'rdp' : 'vnc'
      const label = protocol === 'rdp' ? 'RDP' : 'VNC'
      if (!asset.hasDesktopPassword) {
        try {
          await ElMessageBox.confirm(
            t('desktop.noPasswordMsg', { label }),
            t('desktop.noPasswordTitle', { label }),
            {
              confirmButtonText: t('desktop.goDetail'),
              cancelButtonText: t('common.cancel'),
              type: 'warning'
            }
          )
          openDetail?.(asset)
        } catch {
          /* cancelled */
        }
        return
      }
      openSessionTab(`/sessions/${asset.id}/${protocol}`, protocol, asset.id)
    }
  }
]
