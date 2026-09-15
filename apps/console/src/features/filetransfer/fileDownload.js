import { ElMessage } from 'element-plus'
import { t } from '../../i18n'
import { downloadFile } from './transferClient'

/**
 * Stream a remote file to disk (File System Access when available, else Blob).
 * Returns save info on success; false when cancelled or failed.
 * Does not toast success — caller shows an in-page completion state.
 */
export async function downloadToDisk({ assetId, remotePath, fileName, onProgress }) {
  try {
    return await downloadFile({ assetId, remotePath, fileName, onProgress })
  } catch (e) {
    if (e?.name === 'AbortError') return false
    ElMessage.error(e.message || t('filetransfer.downloadFailed'))
    return false
  }
}
