import { IconFolder } from '@tabler/icons-vue'
import { t } from '../../i18n'
import { openSessionTab } from '../../session/openSessionTab'

export const filemanagerActions = [
  {
    id: 'files',
    type: 'warning',
    icon: IconFolder,
    requiresOnline: true,
    match: () => true,
    title: () => t('actions.files'),
    open: (asset) => {
      openSessionTab(`/sessions/${asset.id}/files`, 'files', asset.id)
    }
  }
]
