import { ElMessage } from 'element-plus'
import { t } from '../../i18n'
import { downloadFile } from './transferClient'

/**
 * Stream a remote file to disk (File System Access when available, else Blob).
 * Reports its own outcome; returns false when cancelled or failed.
 */
export async function downloadToDisk({ assetId, remotePath, fileName, onProgress }) {
  try {
    await downloadFile({ assetId, remotePath, fileName, onProgress })
    ElMessage.success(t('filetransfer.downloadDone'))
    return true
  } catch (e) {
    if (e?.name === 'AbortError') return false
    ElMessage.error(e.message || t('filetransfer.downloadFailed'))
    return false
  }
}
