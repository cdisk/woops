/**
 * Charset detection and round-trip encoding for remote text files.
 *
 * Ops hosts routinely carry GBK/GB18030 configs, so decoding everything as
 * UTF-8 turns Chinese into mojibake. Detection order is BOM → strict UTF-8 →
 * legacy CJK, and saving re-encodes with whatever the file used so editing a
 * config never silently rewrites its encoding.
 */

const UTF8_BOM = [0xef, 0xbb, 0xbf]

/** Legacy candidates tried (in order) when the bytes are not valid UTF-8. */
const LEGACY_CANDIDATES = ['gb18030', 'big5', 'shift_jis']

export const CHARSET_OPTIONS = [
  { label: 'UTF-8', value: 'utf-8' },
  { label: 'GBK / GB18030', value: 'gb18030' },
  { label: 'Big5', value: 'big5' },
  { label: 'Shift_JIS', value: 'shift_jis' },
  { label: 'UTF-16 LE', value: 'utf-16le' },
  { label: 'UTF-16 BE', value: 'utf-16be' },
  { label: 'Latin-1', value: 'windows-1252' }
]

export function charsetLabel(charset) {
  return CHARSET_OPTIONS.find((o) => o.value === charset)?.label || charset || 'UTF-8'
}

function startsWith(bytes, prefix) {
  if (bytes.length < prefix.length) return false
  return prefix.every((b, i) => bytes[i] === b)
}

function isAscii(bytes) {
  for (let i = 0; i < bytes.length; i++) {
    if (bytes[i] > 0x7f) return false
  }
  return true
}

function decodesCleanly(bytes, label) {
  try {
    new TextDecoder(label, { fatal: true }).decode(bytes)
    return true
  } catch {
    return false
  }
}

/** @returns {{charset: string, bom: boolean}} */
export function detectCharset(bytes) {
  if (!bytes || !bytes.length) return { charset: 'utf-8', bom: false }
  if (startsWith(bytes, UTF8_BOM)) return { charset: 'utf-8', bom: true }
  if (bytes.length >= 2 && bytes[0] === 0xff && bytes[1] === 0xfe) return { charset: 'utf-16le', bom: true }
  if (bytes.length >= 2 && bytes[0] === 0xfe && bytes[1] === 0xff) return { charset: 'utf-16be', bom: true }
  if (isAscii(bytes)) return { charset: 'utf-8', bom: false }
  if (decodesCleanly(bytes, 'utf-8')) return { charset: 'utf-8', bom: false }
  for (const label of LEGACY_CANDIDATES) {
    if (decodesCleanly(bytes, label)) return { charset: label, bom: false }
  }
  return { charset: 'windows-1252', bom: false }
}

/** Lenient decode; a BOM matching the charset is dropped by TextDecoder itself. */
export function decodeText(bytes, charset = 'utf-8') {
  try {
    return new TextDecoder(charset, { fatal: false }).decode(bytes)
  } catch {
    return new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  }
}

const reverseTables = new Map()

/**
 * TextEncoder only speaks UTF-8, so derive char → bytes from the matching
 * decoder once per charset. Covers single bytes and the 2-byte planes, which is
 * everything a GBK/Big5/Shift_JIS text file can hold short of GB18030's 4-byte
 * forms; anything left over is reported as unmapped instead of being mangled.
 */
function reverseTable(label) {
  const cached = reverseTables.get(label)
  if (cached) return cached
  const table = new Map()
  const decoder = new TextDecoder(label, { fatal: true })
  const single = new Uint8Array(1)
  for (let b = 0; b <= 0xff; b++) {
    single[0] = b
    try {
      const ch = decoder.decode(single)
      if ([...ch].length === 1 && !table.has(ch)) table.set(ch, [b])
    } catch {
      // byte is only valid as part of a multi-byte sequence
    }
  }
  const pair = new Uint8Array(2)
  for (let lead = 0x81; lead <= 0xfe; lead++) {
    pair[0] = lead
    for (let trail = 0x40; trail <= 0xfe; trail++) {
      pair[1] = trail
      try {
        const ch = decoder.decode(pair)
        if ([...ch].length === 1 && !table.has(ch)) table.set(ch, [lead, trail])
      } catch {
        // invalid pair for this charset
      }
    }
  }
  reverseTables.set(label, table)
  return table
}

function encodeUtf16(text, littleEndian, bom) {
  const units = text.length
  const out = new Uint8Array((units + (bom ? 1 : 0)) * 2)
  const view = new DataView(out.buffer)
  let pos = 0
  if (bom) {
    view.setUint16(0, 0xfeff, littleEndian)
    pos = 2
  }
  for (let i = 0; i < units; i++) {
    view.setUint16(pos, text.charCodeAt(i), littleEndian)
    pos += 2
  }
  return out
}

/**
 * @returns {{bytes: Uint8Array, unmapped: string[]}} `unmapped` lists characters
 * the target charset cannot represent (written as `?`), so callers can offer
 * saving as UTF-8 instead.
 */
export function encodeText(text, charset = 'utf-8', bom = false) {
  const value = String(text ?? '')
  if (!charset || charset === 'utf-8') {
    const body = new TextEncoder().encode(value)
    if (!bom) return { bytes: body, unmapped: [] }
    const out = new Uint8Array(UTF8_BOM.length + body.length)
    out.set(UTF8_BOM, 0)
    out.set(body, UTF8_BOM.length)
    return { bytes: out, unmapped: [] }
  }
  if (charset === 'utf-16le' || charset === 'utf-16be') {
    return { bytes: encodeUtf16(value, charset === 'utf-16le', bom), unmapped: [] }
  }
  const table = reverseTable(charset)
  const out = []
  const unmapped = new Set()
  for (const ch of value) {
    const seq = table.get(ch)
    if (seq) {
      for (const b of seq) out.push(b)
    } else {
      unmapped.add(ch)
      out.push(0x3f)
    }
  }
  return { bytes: Uint8Array.from(out), unmapped: [...unmapped] }
}
