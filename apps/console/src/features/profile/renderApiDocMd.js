/**
 * Minimal Markdown → safe HTML for API docs preview (no external deps).
 * Supports headings, paragraphs, lists, fenced code, tables, inline code/bold/links.
 * Returns { html, toc } where toc is [{ id, level, text }].
 */
export function renderApiDoc(md) {
  if (!md) return { html: '', toc: [] }
  const lines = md.replace(/\r\n/g, '\n').split('\n')
  const html = []
  const toc = []
  const usedIds = new Set()
  let i = 0
  let inCode = false
  let codeLang = ''
  let codeBuf = []
  let inUl = false
  let inTable = false
  let tableRows = []

  const closeUl = () => {
    if (inUl) {
      html.push('</ul>')
      inUl = false
    }
  }
  const flushTable = () => {
    if (!inTable) return
    html.push('<table>')
    tableRows.forEach((cells, idx) => {
      const tag = idx === 0 ? 'th' : 'td'
      if (idx === 1 && cells.every((c) => /^:?-+:?$/.test(c.trim()))) return
      html.push('<tr>')
      cells.forEach((c) => html.push(`<${tag}>${inline(c.trim())}</${tag}>`))
      html.push('</tr>')
    })
    html.push('</table>')
    inTable = false
    tableRows = []
  }

  const pushHeading = (level, raw) => {
    const text = stripMd(raw)
    const id = uniqueId(slugify(text), usedIds)
    toc.push({ id, level, text })
    html.push(`<h${level} id="${esc(id)}">${inline(raw)}</h${level}>`)
  }

  while (i < lines.length) {
    const line = lines[i]
    if (inCode) {
      if (line.startsWith('```')) {
        html.push(`<pre><code class="lang-${esc(codeLang)}">${esc(codeBuf.join('\n'))}</code></pre>`)
        inCode = false
        codeBuf = []
        codeLang = ''
      } else {
        codeBuf.push(line)
      }
      i++
      continue
    }
    if (line.startsWith('```')) {
      closeUl()
      flushTable()
      inCode = true
      codeLang = line.slice(3).trim()
      i++
      continue
    }
    if (line.startsWith('|')) {
      closeUl()
      inTable = true
      tableRows.push(line.replace(/^\|/, '').replace(/\|$/, '').split('|'))
      i++
      continue
    }
    if (inTable) flushTable()

    if (/^\s*[-*] /.test(line)) {
      if (!inUl) {
        html.push('<ul>')
        inUl = true
      }
      html.push(`<li>${inline(line.replace(/^\s*[-*] /, ''))}</li>`)
      i++
      continue
    }
    closeUl()

    if (line.startsWith('### ')) {
      pushHeading(3, line.slice(4))
    } else if (line.startsWith('## ')) {
      pushHeading(2, line.slice(3))
    } else if (line.startsWith('# ')) {
      pushHeading(1, line.slice(2))
    } else if (line.trim() === '') {
      // skip
    } else {
      html.push(`<p>${inline(line)}</p>`)
    }
    i++
  }
  closeUl()
  flushTable()
  if (inCode) {
    html.push(`<pre><code>${esc(codeBuf.join('\n'))}</code></pre>`)
  }
  return { html: html.join('\n'), toc }
}

/** @deprecated use renderApiDoc */
export function renderApiDocHtml(md) {
  return renderApiDoc(md).html
}

function stripMd(s) {
  return String(s)
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .trim()
}

function slugify(text) {
  const base = text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
  return base || 'section'
}

function uniqueId(base, used) {
  let id = base
  let n = 2
  while (used.has(id)) {
    id = `${base}-${n++}`
  }
  used.add(id)
  return id
}

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function inline(s) {
  let out = esc(s)
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>')
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  out = out.replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
  return out
}
