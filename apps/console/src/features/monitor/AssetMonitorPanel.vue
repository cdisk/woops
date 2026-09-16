<template>
  <div class="monitor-panel" v-loading="loading">
    <div class="toolbar">
      <div class="title-block">
        <div class="title-row">
          <el-button class="back-btn" @click="goBack">
            <IconArrowLeft :size="20" stroke="1.75" />
            {{ t('common.close') }}
          </el-button>
          <SessionAssetTitle :title="panelTitle" :kind="t('monitor.kind')" :asset="asset" />
        </div>
        <p class="host-specs">{{ hostSpecsText }}</p>
      </div>
      <div class="controls">
        <el-date-picker
          v-model="range"
          type="datetimerange"
          :range-separator="t('monitor.rangeTo')"
          :start-placeholder="t('monitor.start')"
          :end-placeholder="t('monitor.end')"
          :shortcuts="rangeShortcuts"
          size="small"
          @change="handleRangeChange"
        />
        <el-radio-group v-model="grain" size="small" @change="handleGrainChange">
          <el-radio-button value="minute">{{ t('monitor.grainMinute') }}</el-radio-button>
          <el-radio-button value="hour">{{ t('monitor.grainHour') }}</el-radio-button>
          <el-radio-button value="day">{{ t('monitor.grainDay') }}</el-radio-button>
          <el-radio-button value="month">{{ t('monitor.grainMonth') }}</el-radio-button>
        </el-radio-group>
        <el-button size="small" @click="reload">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div class="cards">
      <div class="card" v-for="c in summaryCards" :key="c.key">
        <div class="card-label">{{ c.label }}</div>
        <div class="card-value">{{ c.value }}</div>
      </div>
    </div>

    <div class="charts">
      <p class="brush-hint">{{ t('monitor.brushHint') }}</p>
      <div v-for="ch in chartDefs" :key="ch.itemId" class="chart-box">
        <div class="chart-title">{{ ch.name }}</div>
        <div :ref="(el) => setChartRef(ch.itemId, el)" class="chart"></div>
        <div class="chart-stats">
          <span>{{ t('monitor.max', { v: chartStats[ch.itemId]?.max ?? '-' }) }}</span>
          <span>{{ t('monitor.avg', { v: chartStats[ch.itemId]?.avg ?? '-' }) }}</span>
          <span>{{ t('monitor.min', { v: chartStats[ch.itemId]?.min ?? '-' }) }}</span>
        </div>
      </div>
    </div>

    <el-row :gutter="16" class="tables">
      <el-col :span="12">
        <h3>{{ t('monitor.disks') }}</h3>
        <el-table :data="diskRows" size="small" stripe>
          <el-table-column prop="instance" :label="t('monitor.mount')" min-width="120" />
          <el-table-column prop="usedPercent" :label="t('monitor.usedPercent')" width="90" />
          <el-table-column prop="used" :label="t('monitor.used')" width="100" />
          <el-table-column prop="total" :label="t('monitor.total')" width="100" />
        </el-table>
      </el-col>
      <el-col :span="12">
        <h3>{{ t('monitor.nics') }}</h3>
        <el-table :data="netRows" size="small" stripe>
          <el-table-column prop="instance" :label="t('monitor.nic')" min-width="100" />
          <el-table-column prop="rx" :label="t('monitor.rx')" width="110" />
          <el-table-column prop="tx" :label="t('monitor.tx')" width="110" />
          <el-table-column prop="rxErr" :label="t('monitor.rxErr')" width="70" />
          <el-table-column prop="txErr" :label="t('monitor.txErr')" width="70" />
        </el-table>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { IconArrowLeft } from '@tabler/icons-vue'
import { LineChart } from 'echarts/charts'
import { BrushComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { init, use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import api from '../../shared/api'
import { closeSessionTab } from '../../session/sessionWs'
import SessionAssetTitle from '../../session/SessionAssetTitle.vue'

use([LineChart, BrushComponent, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const { t } = useI18n()

const props = defineProps({
  assetId: { type: String, required: true },
  asset: { type: Object, default: null }
})

const goBack = closeSessionTab

const panelTitle = computed(() => props.asset?.displayName || props.asset?.hostname || t('monitor.title'))

const loading = ref(false)
const latest = ref([])
const grain = ref('minute')
const range = ref([new Date(Date.now() - 24 * 3600 * 1000), new Date()])
const chartDefs = ref([])
const seriesPayload = ref({ series: {} })
const chartEls = {}
const charts = {}
/** Skip radio @change when grain is updated programmatically (range/API). */
let suppressGrainChange = false

function rangeEndingNow(ms) {
  const end = new Date()
  return [new Date(end.getTime() - ms), end]
}

const MS = {
  m: 60 * 1000,
  h: 3600 * 1000,
  d: 24 * 3600 * 1000
}

const rangeShortcuts = computed(() => [
  { text: t('monitor.range5m'), value: () => rangeEndingNow(5 * MS.m) },
  { text: t('monitor.range30m'), value: () => rangeEndingNow(30 * MS.m) },
  { text: t('monitor.range1h'), value: () => rangeEndingNow(MS.h) },
  { text: t('monitor.range6h'), value: () => rangeEndingNow(6 * MS.h) },
  { text: t('monitor.range1d'), value: () => rangeEndingNow(MS.d) },
  { text: t('monitor.range7d'), value: () => rangeEndingNow(7 * MS.d) },
  { text: t('monitor.range30d'), value: () => rangeEndingNow(30 * MS.d) },
  { text: t('monitor.range90d'), value: () => rangeEndingNow(90 * MS.d) },
  { text: t('monitor.range1y'), value: () => rangeEndingNow(365 * MS.d) }
])

function setGrain(next) {
  if (grain.value === next) return
  suppressGrainChange = true
  grain.value = next
  suppressGrainChange = false
}

function setChartRef(id, el) {
  if (el) chartEls[id] = el
}

function latestMap() {
  const m = {}
  for (const row of latest.value) {
    m[`${row.itemId}|${row.instance || ''}`] = row
  }
  return m
}

function pick(itemId, instance = '') {
  return latestMap()[`${itemId}|${instance}`]?.value
}

function fmtPercent(v) {
  if (v == null || Number.isNaN(v)) return '-'
  return `${Number(v).toFixed(1)}%`
}

function fmtBytes(v) {
  if (v == null || Number.isNaN(v)) return '-'
  const n = Number(v)
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let x = n
  while (x >= 1024 && i < u.length - 1) {
    x /= 1024
    i++
  }
  return `${x.toFixed(i === 0 ? 0 : 1)} ${u[i]}`
}

function fmtNum(v) {
  if (v == null || Number.isNaN(v)) return '-'
  return Number(v).toFixed(2)
}

function fmtMhz(v) {
  if (v == null || Number.isNaN(v) || v <= 0) return null
  const n = Number(v)
  if (n >= 1000) return `${(n / 1000).toFixed(2)} GHz`
  return `${Math.round(n)} MHz`
}

const hostSpecsText = computed(() => {
  const parts = []
  const logical = pick('cpu.count_logical')
  const physical = pick('cpu.count_physical')
  if (logical != null) {
    const cores = Math.round(logical)
    if (physical != null && Math.round(physical) > 0 && Math.round(physical) !== cores) {
      parts.push(t('monitor.physCores', { phys: Math.round(physical), logical: cores }))
    } else {
      parts.push(t('monitor.cores', { n: cores }))
    }
  }
  const mhz = fmtMhz(pick('cpu.mhz'))
  if (mhz) parts.push(mhz)

  const memTot = pick('mem.total_bytes')
  if (memTot != null) parts.push(t('monitor.memSpec', { size: fmtBytes(memTot) }))

  const swapTot = pick('swap.total_bytes')
  if (swapTot != null && swapTot > 0) parts.push(`Swap ${fmtBytes(swapTot)}`)

  const disks = []
  for (const row of latest.value) {
    if (row.itemId !== 'disk.total_bytes' || !row.instance) continue
    disks.push(`${row.instance} ${fmtBytes(row.value)}`)
  }
  if (disks.length) parts.push(t('monitor.diskSpec', { list: disks.join(', ') }))

  return parts.length ? parts.join(' · ') : t('monitor.noHostSpec')
})

const summaryCards = computed(() => [
  { key: 'cpu', label: 'CPU', value: fmtPercent(pick('cpu.usage_percent')) },
  { key: 'mem', label: t('monitor.mem'), value: fmtPercent(pick('mem.used_percent')) },
  { key: 'swap', label: 'Swap', value: fmtPercent(pick('swap.used_percent')) },
  { key: 'load', label: 'Load 1m', value: fmtNum(pick('load.load1')) },
  { key: 'proc', label: t('monitor.processes'), value: pick('process.count') != null ? String(Math.round(pick('process.count'))) : '-' },
  {
    key: 'uptime',
    label: t('monitor.uptime'),
    value: (() => {
      const s = pick('host.uptime_sec')
      if (s == null) return '-'
      const d = Math.floor(s / 86400)
      const h = Math.floor((s % 86400) / 3600)
      return d > 0 ? t('monitor.uptimeDh', { d, h }) : t('monitor.uptimeH', { h })
    })()
  }
])

const diskRows = computed(() => {
  const by = {}
  for (const row of latest.value) {
    if (!row.itemId?.startsWith('disk.')) continue
    const inst = row.instance || ''
    if (!inst) continue
    by[inst] ||= { instance: inst }
    if (row.itemId === 'disk.used_percent') by[inst].usedPercent = fmtPercent(row.value)
    if (row.itemId === 'disk.used_bytes') by[inst].used = fmtBytes(row.value)
    if (row.itemId === 'disk.total_bytes') by[inst].total = fmtBytes(row.value)
  }
  return Object.values(by)
})

const netRows = computed(() => {
  const by = {}
  for (const row of latest.value) {
    if (!row.itemId?.startsWith('net.')) continue
    const inst = row.instance || ''
    if (!inst) continue
    by[inst] ||= { instance: inst }
    if (row.itemId === 'net.rx_bytes_per_sec') by[inst].rx = fmtBytes(row.value)
    if (row.itemId === 'net.tx_bytes_per_sec') by[inst].tx = fmtBytes(row.value)
    if (row.itemId === 'net.rx_errors') by[inst].rxErr = Math.round(row.value)
    if (row.itemId === 'net.tx_errors') by[inst].txErr = Math.round(row.value)
  }
  return Object.values(by)
})

async function loadLatest() {
  const { data } = await api.get(`/assets/${props.assetId}/metrics/latest`)
  latest.value = data || []
}

async function loadSeries() {
  const [from, to] = range.value || []
  const params = {
    grain: grain.value,
    from: from ? new Date(from).toISOString() : undefined,
    to: to ? new Date(to).toISOString() : undefined
  }
  const keys = CHART_ORDER.join(',')
  if (keys) params.keys = keys
  const { data } = await api.get(`/assets/${props.assetId}/metrics/series`, { params })
  seriesPayload.value = data || { series: {} }
  // Only sync UI when API had to rewrite (e.g. trends has no minute).
  if (data?.grain && data.grain !== grain.value) {
    setGrain(data.grain)
  }
  await nextTick()
  renderCharts()
}

async function withPanelLoading(task) {
  loading.value = true
  try {
    await task()
  } finally {
    loading.value = false
  }
}

/** Display order: CPU → mem → disk → Load → Swap → nic rx/tx */
const CHART_ORDER = [
  'cpu.usage_percent',
  'mem.used_percent',
  'disk.used_percent',
  'load.load1',
  'swap.used_percent',
  'net.rx_bytes_per_sec',
  'net.tx_bytes_per_sec'
]

function recommendedGrain() {
  const [from, to] = range.value || []
  if (!from || !to) return grain.value
  const spanMs = Math.max(0, new Date(to).getTime() - new Date(from).getTime())
  // Defaults by rough point budget (manual choice still wins):
  // minute ≤3d; hour ≤90d (~2k pts); day for longer incl. 1y (~365 pts); month only multi-year.
  if (spanMs > 3 * 365 * MS.d) return 'month'
  if (spanMs > 90 * MS.d) return 'day'
  if (spanMs > 3 * MS.d) return 'hour'
  return 'minute'
}

async function handleRangeChange() {
  setGrain(recommendedGrain())
  await withPanelLoading(loadSeries)
}

async function handleGrainChange() {
  if (suppressGrainChange) return
  await withPanelLoading(loadSeries)
}

async function loadItemDefs() {
  const { data } = await api.get('/monitor/items')
  const byId = Object.fromEntries((data || []).map((d) => [d.itemId, d]))
  chartDefs.value = CHART_ORDER.map((id) => byId[id]).filter(Boolean)
}

function formatChartStat(v, unit) {
  if (v == null || Number.isNaN(v)) return '-'
  if (unit === 'bytes_per_sec' || unit === 'bytes') return fmtBytes(v)
  const n = Number(v).toFixed(2)
  return unit === 'percent' ? `${n}%` : n
}

const chartStats = computed(() => {
  const seriesMap = seriesPayload.value.series || {}
  const summaries = seriesPayload.value.summaries || {}
  const out = {}
  for (const ch of chartDefs.value) {
    // Prefer server summaries (stable across grain); aggregate multi-instance by sampleCount.
    let min = null
    let max = null
    let weighted = 0
    let n = 0
    for (const [k, sum] of Object.entries(summaries)) {
      if (k !== ch.itemId && !k.startsWith(ch.itemId + '|')) continue
      const sc = Number(sum?.sampleCount) || 0
      if (sc <= 0 || sum?.avg == null) continue
      const sMin = Number(sum.min)
      const sMax = Number(sum.max)
      const sAvg = Number(sum.avg)
      if (!Number.isNaN(sMin)) min = min == null ? sMin : Math.min(min, sMin)
      if (!Number.isNaN(sMax)) max = max == null ? sMax : Math.max(max, sMax)
      if (!Number.isNaN(sAvg)) {
        weighted += sAvg * sc
        n += sc
      }
    }
    if (n > 0) {
      out[ch.itemId] = {
        max: formatChartStat(max, ch.unit),
        avg: formatChartStat(weighted / n, ch.unit),
        min: formatChartStat(min, ch.unit)
      }
      continue
    }
    // Fallback for older API without summaries.
    const vals = []
    for (const [k, pts] of Object.entries(seriesMap)) {
      if (k !== ch.itemId && !k.startsWith(ch.itemId + '|')) continue
      for (const p of pts || []) {
        const v = Number(p.value)
        if (!Number.isNaN(v)) vals.push(v)
      }
    }
    if (!vals.length) {
      out[ch.itemId] = { max: '-', avg: '-', min: '-' }
      continue
    }
    const vmax = Math.max(...vals)
    const vmin = Math.min(...vals)
    const vavg = vals.reduce((a, b) => a + b, 0) / vals.length
    out[ch.itemId] = {
      max: formatChartStat(vmax, ch.unit),
      avg: formatChartStat(vavg, ch.unit),
      min: formatChartStat(vmin, ch.unit)
    }
  }
  return out
})

function clearChartBrushes() {
  Object.values(charts).forEach((c) => {
    c.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
  })
}

function enableChartBrush(chart) {
  chart.dispatchAction({
    type: 'takeGlobalCursor',
    key: 'brush',
    brushOption: { brushType: 'lineX', brushMode: 'single' }
  })
}

function bindChartBrush(chart) {
  if (chart.__monitorBrushBound) return
  chart.__monitorBrushBound = true
  chart.on('brushEnd', (params) => {
    void applyBrushRange(params)
  })
}

function parseBrushTime(v) {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (Array.isArray(v) && v.length >= 1) return parseBrushTime(v[0])
  const t = new Date(v).getTime()
  return Number.isFinite(t) ? t : NaN
}

async function applyBrushRange(params) {
  if (loading.value) return
  const area = params?.areas?.[0]
  if (!area?.coordRange || area.coordRange.length < 2) return
  let t0 = parseBrushTime(area.coordRange[0])
  let t1 = parseBrushTime(area.coordRange[1])
  if (!Number.isFinite(t0) || !Number.isFinite(t1)) return
  if (t0 > t1) [t0, t1] = [t1, t0]
  // Ignore tiny accidental drags (< 1 minute).
  if (t1 - t0 < MS.m) {
    clearChartBrushes()
    Object.values(charts).forEach(enableChartBrush)
    return
  }
  range.value = [new Date(t0), new Date(t1)]
  clearChartBrushes()
  setGrain(recommendedGrain())
  await withPanelLoading(loadSeries)
}

function renderCharts() {
  const seriesMap = seriesPayload.value.series || {}
  for (const ch of chartDefs.value) {
    const el = chartEls[ch.itemId]
    if (!el) continue
    if (!charts[ch.itemId]) {
      charts[ch.itemId] = init(el)
    }
    const chart = charts[ch.itemId]
    bindChartBrush(chart)
    const seriesKeys = Object.keys(seriesMap).filter((k) => k === ch.itemId || k.startsWith(ch.itemId + '|'))
    const series = seriesKeys.map((k) => {
      const pts = seriesMap[k] || []
      const inst = k.includes('|') ? k.split('|').slice(1).join('|') : ''
      return {
        name: inst || ch.name,
        type: 'line',
        showSymbol: false,
        data: pts.map((p) => [p.time, p.value])
      }
    })
    chart.setOption({
      tooltip: {
        trigger: 'axis',
        valueFormatter: (v) => {
          if (v == null || Number.isNaN(Number(v))) return '-'
          const n = Number(v).toFixed(2)
          return ch.unit === 'percent' ? `${n}%` : n
        }
      },
      brush: {
        toolbox: [],
        xAxisIndex: 0,
        brushLink: 'all',
        brushStyle: {
          borderWidth: 1,
          color: 'rgba(64, 158, 255, 0.12)',
          borderColor: '#409eff'
        },
        throttleType: 'debounce',
        throttleDelay: 100
      },
      legend: { type: 'scroll', top: 0 },
      grid: { left: 48, right: 16, top: 36, bottom: 28 },
      xAxis: { type: 'time' },
      yAxis: {
        type: 'value',
        axisLabel: {
          formatter: (v) => {
            const n = Number(v).toFixed(2)
            return ch.unit === 'percent' ? `${n}%` : n
          }
        }
      },
      series: series.length ? series : [{ type: 'line', data: [] }]
    }, true)
    enableChartBrush(chart)
  }
}

async function reload() {
  loading.value = true
  try {
    await Promise.all([loadLatest(), loadSeries()])
  } finally {
    loading.value = false
  }
}

function onResize() {
  Object.values(charts).forEach((c) => c.resize())
}

watch(() => props.assetId, () => reload())

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([loadItemDefs(), loadLatest(), loadSeries()])
    await nextTick()
    renderCharts()
  } finally {
    loading.value = false
  }
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  Object.values(charts).forEach((c) => c.dispose())
})
</script>

<style scoped>
.monitor-panel { display: flex; flex-direction: column; gap: 16px; }
.toolbar { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; align-items: flex-start; }
.title-row { display: flex; align-items: center; gap: 12px; }
.title-row :deep(.session-titles) { flex: 1; min-width: 0; }
.title-row :deep(.title-line) { font-size: 18px; }
.back-btn {
  font-size: 15px;
  height: 36px;
  padding: 0 14px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.host-specs { margin: 8px 0 0; color: #111827; font-size: 13px; line-height: 1.5; word-break: break-all; }
.controls { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
.card { background: #f8fafc; border: 1px solid #e5e7eb; border-radius: 8px; padding: 12px; }
.card-label { color: #6b7280; font-size: 12px; }
.card-value { margin-top: 6px; font-size: 20px; font-weight: 600; }
.charts { display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 12px; }
.brush-hint {
  grid-column: 1 / -1;
  margin: 0;
  font-size: 12px;
  color: #6b7280;
}
.chart-box { border: 1px solid #e5e7eb; border-radius: 8px; padding: 8px 8px 4px; }
.chart-title { font-size: 13px; color: #374151; padding: 0 4px 4px; }
.chart { height: 220px; width: 100%; }
.chart-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  padding: 4px 8px 6px;
  font-size: 12px;
  color: #6b7280;
}
.tables h3 { margin: 0 0 8px; font-size: 14px; }
</style>
