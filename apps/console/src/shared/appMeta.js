import pkg from '../../package.json'

/** Console build version (package.json). Bump when shipping a named release. */
export const APP_VERSION = pkg.version || '0.0.0'

/** Public source repository. */
export const APP_REPO_URL = 'https://gitee.com/cdisk/woops'

export const APP_REPO_HOST_PATH = APP_REPO_URL.replace(/^https?:\/\//, '')
