import { IconChartAreaLine } from '@tabler/icons-vue'
import { t } from '../../i18n'
import { openSessionTab } from '../../session/openSessionTab'

export const monitorActions = [
  {
    id: 'monitor',
    type: 'success',
    icon: IconChartAreaLine,
    requiresOnline: false,
    match: () => true,
    title: () => t('actions.monitor'),
    open: (asset) => {
      openSessionTab(`/assets/${asset.id}/monitor`, 'monitor', asset.id)
    }
  }
]
