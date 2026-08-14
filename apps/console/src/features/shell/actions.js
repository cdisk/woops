import { IconTerminal2 } from '@tabler/icons-vue'
import { isWindows } from '../../session/assetOs'
import { openSessionTab } from '../../session/openSessionTab'

export const shellActions = [
  {
    id: 'shell',
    type: 'primary',
    icon: IconTerminal2,
    requiresOnline: true,
    match: () => true,
    title: (asset) => (isWindows(asset) ? 'PowerShell' : 'Bash'),
    open: (asset) => {
      const kind = isWindows(asset) ? 'powershell' : 'bash'
      openSessionTab(`/sessions/${asset.id}/shell?kind=${kind}`, `shell-${kind}`, asset.id)
    }
  }
]
