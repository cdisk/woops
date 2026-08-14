/** Text / config extensions editable in the file manager. */
const TEXT_EXT = new Set([
  'sh', 'bash', 'zsh', 'ksh', 'csh', 'fish',
  'txt', 'text', 'log', 'md', 'markdown', 'rst',
  'json', 'jsonc', 'yaml', 'yml', 'toml', 'ini', 'conf', 'cfg', 'cnf', 'properties',
  'env', 'service', 'timer', 'socket', 'mount', 'path', 'target', 'desktop',
  'xml', 'html', 'htm', 'css', 'scss', 'less',
  'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'vue',
  'py', 'rb', 'php', 'go', 'rs', 'java', 'kt', 'c', 'h', 'cc', 'cpp', 'hpp',
  'sql', 'csv', 'tsv', 'gitignore', 'dockerignore', 'editorconfig', 'npmrc', 'yarnrc'
])

const TEXT_BASENAMES = new Set([
  'dockerfile', 'makefile', 'gemfile', 'procfile', 'jenkinsfile',
  '.bashrc', '.bash_profile', '.profile', '.zshrc', '.gitignore', '.dockerignore',
  '.env', '.editorconfig', 'readme', 'license', 'changelog'
])

/** Shell-highlight extensions / basenames (more modes can be added later). */
const SHELL_EXT = new Set(['sh', 'bash', 'zsh', 'ksh', 'csh', 'fish', 'service', 'timer', 'socket', 'mount', 'path', 'target'])
const SHELL_BASENAMES = new Set(['.bashrc', '.bash_profile', '.profile', '.zshrc', 'dockerfile'])

const MAX_EDIT_BYTES = 2 * 1024 * 1024

export function maxEditBytes() {
  return MAX_EDIT_BYTES
}

export function fileBaseName(name) {
  const n = String(name || '')
  const i = Math.max(n.lastIndexOf('/'), n.lastIndexOf('\\'))
  return i >= 0 ? n.slice(i + 1) : n
}

export function fileExt(name) {
  const base = fileBaseName(name)
  const i = base.lastIndexOf('.')
  if (i <= 0) return ''
  return base.slice(i + 1).toLowerCase()
}

export function isEditableTextFile(name) {
  const base = fileBaseName(name).toLowerCase()
  if (TEXT_BASENAMES.has(base)) return true
  const ext = fileExt(name)
  return !!ext && TEXT_EXT.has(ext)
}

/** Highlight mode id for CodeMirror; only shell for now, else plain. */
export function editorLanguageFor(name) {
  const base = fileBaseName(name).toLowerCase()
  if (SHELL_BASENAMES.has(base)) return 'shell'
  const ext = fileExt(name)
  if (SHELL_EXT.has(ext)) return 'shell'
  return 'plain'
}

/**
 * NUL bytes and control-character density mean binary — except under UTF-16,
 * where ASCII text is half NUL bytes and the BOM already proved it is text.
 */
export function looksBinary(bytes, charset = '') {
  if (!bytes || !bytes.length) return false
  if (charset === 'utf-16le' || charset === 'utf-16be') return false
  const n = Math.min(bytes.length, 8000)
  let weird = 0
  for (let i = 0; i < n; i++) {
    const b = bytes[i]
    if (b === 0) return true
    if (b < 7 || (b > 13 && b < 32)) weird++
  }
  return weird / n > 0.3
}
