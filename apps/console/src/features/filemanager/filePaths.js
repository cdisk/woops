/** Path helpers for Agent filemanager (Windows drives + Unix). */

export function joinPath(dir, name, isWindows) {
  if (!dir || dir === 'roots') {
    if (/^[A-Za-z]:$/.test(name)) return `${name}\\`
    return isWindows ? `${name}\\` : `/${name}`.replace(/\/+/g, '/')
  }
  const base = String(dir).replace(/[/\\]+$/, '')
  if (/^[A-Za-z]:/.test(base)) {
    return `${base}\\${name}`
  }
  if (base === '/' || base === '') return `/${name}`.replace(/\/+/g, '/')
  return `${base}/${name}`.replace(/\/+/g, '/')
}

export function joinRelative(dir, relative, isWindows) {
  const parts = String(relative || '').replace(/\\/g, '/').split('/').filter(Boolean)
  let p = dir || '/'
  for (const part of parts) p = joinPath(p, part, isWindows)
  return p
}

export function parentPath(dir, isWindows) {
  const p = String(dir || '').replace(/[/\\]+$/, '')
  if (!p || p === '/' || p === 'roots') return isWindows ? 'roots' : '/'
  if (/^[A-Za-z]:$/.test(p)) return 'roots'
  if (/^[A-Za-z]:/.test(p)) {
    const i = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'))
    if (i <= 2) return p.slice(0, 2) + '\\'
    return p.slice(0, i)
  }
  const i = p.lastIndexOf('/')
  return i <= 0 ? '/' : p.slice(0, i)
}

export function sortEntries(list) {
  return [...(list || [])].sort((a, b) => {
    if (!!a.isDir !== !!b.isDir) return a.isDir ? -1 : 1
    return String(a.name || '').localeCompare(String(b.name || ''), undefined, { sensitivity: 'base' })
  })
}
