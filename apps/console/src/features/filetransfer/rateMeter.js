/**
 * Sliding-window throughput meter.
 *
 * Progress callbacks report an absolute offset, which jumps to the remote offset
 * when a transfer resumes. Averaging over the whole task would count those bytes
 * as if they were sent instantly, so the window only ever measures deltas
 * observed after the baseline sample.
 */

const WINDOW_MS = 5000
const MIN_SPAN_MS = 400
const MIN_STEP_MS = 200

export function createRateMeter() {
  return { samples: [], speed: 0 }
}

export function resetRateMeter(meter, loaded = 0, ts = Date.now()) {
  meter.samples = [{ t: ts, loaded: Number(loaded) || 0 }]
  meter.speed = 0
  return meter.speed
}

export function sampleRate(meter, loaded, ts = Date.now()) {
  const value = Number(loaded) || 0
  const s = meter.samples
  if (!s.length || value < s[s.length - 1].loaded || ts < s[s.length - 1].t) {
    return resetRateMeter(meter, value, ts)
  }
  // Keep samples ~MIN_STEP_MS apart by advancing the newest one, otherwise a
  // chunk-per-millisecond transfer would either flood the buffer or pin the
  // window open so it never slides.
  if (s.length > 1 && ts - s[s.length - 2].t < MIN_STEP_MS) {
    const last = s[s.length - 1]
    last.t = ts
    last.loaded = value
  } else {
    s.push({ t: ts, loaded: value })
  }
  while (s.length > 2 && ts - s[0].t > WINDOW_MS) s.shift()
  const first = s[0]
  const tail = s[s.length - 1]
  const span = tail.t - first.t
  if (span >= MIN_SPAN_MS) {
    meter.speed = Math.max(0, ((tail.loaded - first.loaded) * 1000) / span)
  }
  return meter.speed
}
