import { IconTerminal2 } from '@tabler/icons-vue'
import { isLegacyWindows, isWindows } from '../../session/assetOs'
import { openSessionTab } from '../../session/openSessionTab'

function shellKindForAsset(asset) {
  if (!isWindows(asset)) return 'bash'
  return isLegacyWindows(asset) ? 'cmd' : 'powershell'
}

export const shellActions = [
  {
    id: 'shell',
    type: 'primary',
    icon: IconTerminal2,
    requiresOnline: true,
    match: () => true,
    title: (asset) => {
      const kind = shellKindForAsset(asset)
      if (kind === 'cmd') return 'CMD'
      if (kind === 'powershell') return 'PowerShell'
      return 'Bash'
    },
    open: (asset) => {
      const kind = shellKindForAsset(asset)
      openSessionTab(`/sessions/${asset.id}/shell?kind=${kind}`, `shell-${kind}`, asset.id)
    }
  }
]
