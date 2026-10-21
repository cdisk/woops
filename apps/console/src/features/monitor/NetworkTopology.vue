<template>
  <div class="network-topology">
    <div ref="canvasRef" class="topology-canvas" />

    <div class="topology-controls">
      <el-input
        v-model="query"
        clearable
        size="small"
        :placeholder="t('home.topologySearch')"
        @input="applySearch"
        @keyup.enter="focusFirstMatch"
      />
      <el-tooltip :content="t('home.topologyFit')" placement="bottom">
        <button type="button" class="control-btn" @click="fit">
          <IconArrowsMaximize :size="16" stroke="1.75" />
        </button>
      </el-tooltip>
      <el-tooltip :content="t('home.topologyRelayout')" placement="bottom">
        <button type="button" class="control-btn" @click="runLayout">
          <IconHierarchy3 :size="16" stroke="1.75" />
        </button>
      </el-tooltip>
    </div>

    <div class="topology-legend">
      <span><i class="dot normal" />{{ t('home.topologyNormal') }}</span>
      <span><i class="dot abnormal" />{{ t('home.topologyAbnormal') }}</span>
      <span><i class="dot offline" />{{ t('common.offline') }}</span>
      <span><i class="trunk" />{{ t('home.topologyTrunk') }}</span>
    </div>

    <div
      v-if="hover"
      class="node-tip"
      :style="{ left: `${hover.x}px`, top: `${hover.y}px` }"
    >
      <strong>{{ hover.name }}</strong>
      <span v-if="hover.ip">{{ hover.ip }}</span>
      <span v-if="hover.group">{{ hover.group }}</span>
    </div>

    <div v-if="selected" class="node-popover">
      <button type="button" class="popover-close" @click="clearSelection">×</button>
      <div class="node-title">
        <span class="status-dot" :class="healthClass(selected)" />
        <strong>{{ selected.name || selected.id }}</strong>
      </div>
      <div class="node-meta">{{ selected.privateIp || t('common.emDash') }}</div>
      <div class="node-meta">
        {{ selected.groupName || t('common.ungrouped') }}
        · Agent {{ selected.agentVersion || t('common.emDash') }}
      </div>
      <div v-if="selected.issueCount" class="node-alert">
        {{ t('home.topologyIssueCount', { n: selected.issueCount }) }}
      </div>
      <div class="node-actions">
        <el-button link type="primary" @click="emit('open-asset', selected)">{{ t('common.detail') }}</el-button>
        <el-button link type="primary" @click="emit('open-monitor', selected)">{{ t('home.openMonitor') }}</el-button>
      </div>
    </div>

    <div v-if="!loading && !assetNodes.length" class="topology-empty">
      {{ t('home.topologyEmpty') }}
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { IconArrowsMaximize, IconHierarchy3 } from '@tabler/icons-vue'
import cytoscape from 'cytoscape'

const props = defineProps({
  topology: { type: Object, default: () => ({ nodes: [], edges: [] }) },
  groups: { type: Array, default: () => [] },
  abnormalAssets: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  insets: { type: Object, default: () => ({ top: 130, right: 24, bottom: 56, left: 24 }) }
})
const emit = defineEmits(['open-asset', 'open-monitor', 'ready'])

const GATEWAY_ID = 'gateway'
const UNGROUPED = 'cluster:none'
const NODE = 58
const SLOT = 74
const LEVEL = 92
const GRID_LEVEL = 74
const CLUSTER_PAD = 36
const CLUSTER_GAP = 28
const LABEL_UNITS = 4.6
const LABEL_LINES = 2
const PALETTE = [
  ['#2f6fb5', '#eaf2fb'],
  ['#2f8a5f', '#e9f6ef'],
  ['#8a5cc2', '#f2ecfa'],
  ['#c2702d', '#fbf0e6'],
  ['#2a8d9c', '#e6f5f7'],
  ['#b5486a', '#faeaf0'],
  ['#6b7f2a', '#f1f5e4'],
  ['#4f5fbf', '#ecedfa'],
  ['#9c6b2a', '#f7efe3'],
  ['#3d7f7a', '#e6f2f1']
]

const { t } = useI18n()
const canvasRef = ref(null)
const query = ref('')
const selectedId = ref('')
const hover = ref(null)
let cy = null
let resizeObserver = null

const abnormalById = computed(() => {
  const out = new Map()
  for (const asset of props.abnormalAssets || []) {
    if (asset?.assetId) out.set(String(asset.assetId), asset)
  }
  return out
})

const assetNodes = computed(() =>
  (props.topology?.nodes || []).filter((node) => node.kind === 'asset'))

const selected = computed(() => {
  if (!selectedId.value) return null
  const node = assetNodes.value.find((item) => String(item.id) === selectedId.value)
  if (!node) return null
  const abnormal = abnormalById.value.get(selectedId.value)
  return {
    ...node,
    issueCount: Array.isArray(abnormal?.issues) ? abnormal.issues.length : (abnormal ? 1 : 0)
  }
})

function healthClass(node) {
  if (!node?.online) return 'offline'
  return abnormalById.value.has(String(node.id)) ? 'abnormal' : 'normal'
}

function groupPaths() {
  const out = new Map()
  const walk = (nodes, prefix) => {
    for (const group of nodes || []) {
      const path = prefix ? `${prefix} / ${group.name || ''}` : (group.name || '')
      out.set(String(group.id), path)
      walk(group.children || [], path)
    }
  }
  walk(props.groups, '')
  return out
}

function charUnits(ch) {
  return /[\x00-\xff]/.test(ch) ? 0.6 : 1
}

/** Keeps the tail of the name (the distinguishing part) and wraps it into the square. */
function tailLabel(name) {
  const chars = Array.from(String(name || ''))
  const budget = LABEL_UNITS * LABEL_LINES - 0.6
  let used = 0
  let start = chars.length
  while (start > 0 && used + charUnits(chars[start - 1]) <= budget) {
    used += charUnits(chars[start - 1])
    start -= 1
  }
  let kept = chars.slice(start)
  if (start > 0) {
    while (kept.length && used + 1 > budget) {
      used -= charUnits(kept[0])
      kept = kept.slice(1)
    }
    kept = ['…', ...kept]
  }
  const units = kept.map(charUnits)
  const total = units.reduce((sum, value) => sum + value, 0)
  if (total <= LABEL_UNITS) return kept.join('')
  let best = 1
  let bestWidest = Infinity
  let acc = 0
  for (let i = 1; i < kept.length; i += 1) {
    acc += units[i - 1]
    const widest = Math.max(acc, total - acc)
    if (widest < bestWidest) {
      bestWidest = widest
      best = i
    }
  }
  return `${kept.slice(0, best).join('')}\n${kept.slice(best).join('')}`
}

function nodeLabel(node) {
  if (node.kind === 'gateway') return 'Gateway'
  if (node.kind === 'hidden' || node.hidden) return tailLabel(t('assets.topologyHidden'))
  return tailLabel(node.name || node.privateIp || node.id)
}

function buildModel() {
  const nodes = new Map()
  for (const node of props.topology?.nodes || []) nodes.set(String(node.id), node)
  const upstreamOf = new Map()
  for (const edge of props.topology?.edges || []) {
    upstreamOf.set(String(edge.from), String(edge.to))
  }
  const paths = groupPaths()
  const clusterOf = new Map()
  const clusters = new Map()
  const ensureCluster = (key, label) => {
    if (!clusters.has(key)) clusters.set(key, { key, label, members: [] })
    return clusters.get(key)
  }
  for (const node of nodes.values()) {
    if (node.kind !== 'asset') continue
    const key = node.groupId ? `cluster:${node.groupId}` : UNGROUPED
    const label = node.groupId
      ? (paths.get(String(node.groupId)) || node.groupName || '')
      : t('common.ungrouped')
    ensureCluster(key, label).members.push(String(node.id))
    clusterOf.set(String(node.id), key)
  }
  for (const node of nodes.values()) {
    if (node.kind !== 'hidden') continue
    const id = String(node.id)
    let key = UNGROUPED
    for (const [child, upstream] of upstreamOf) {
      if (upstream === id && clusterOf.has(child)) {
        key = clusterOf.get(child)
        break
      }
    }
    ensureCluster(key, t('common.ungrouped')).members.push(id)
    clusterOf.set(id, key)
  }
  const sortedClusters = [...clusters.values()].sort((a, b) =>
    String(a.label).localeCompare(String(b.label), 'zh'))
  sortedClusters.forEach((cluster, index) => {
    cluster.color = PALETTE[index % PALETTE.length]
  })
  return { nodes, upstreamOf, clusterOf, clusters: sortedClusters }
}

function nodeName(model, id) {
  const node = model.nodes.get(id)
  return String(node?.name || node?.privateIp || id)
}

/** Local coordinates: b = breadth slot, d = depth level, measured from the gateway side. */
function layoutCluster(model, cluster) {
  const members = new Set(cluster.members)
  const children = new Map()
  const roots = []
  const byName = (a, b) => nodeName(model, a).localeCompare(nodeName(model, b), 'zh')
  for (const id of cluster.members) {
    const upstream = model.upstreamOf.get(id)
    if (upstream && members.has(upstream)) {
      if (!children.has(upstream)) children.set(upstream, [])
      children.get(upstream).push(id)
    } else {
      roots.push(id)
    }
  }
  for (const list of children.values()) list.sort(byName)
  roots.sort(byName)

  const local = new Map()
  let slot = 0
  const visiting = new Set()
  const place = (id, depth) => {
    visiting.add(id)
    const kids = (children.get(id) || []).filter((kid) => !visiting.has(kid))
    let b
    if (!kids.length) {
      b = slot
      slot += 1
    } else {
      const placed = kids.map((kid) => place(kid, depth + 1))
      b = (placed[0] + placed[placed.length - 1]) / 2
    }
    local.set(id, { b, d: depth })
    return b
  }
  const trees = roots.filter((id) => (children.get(id) || []).length)
  const singles = roots.filter((id) => !(children.get(id) || []).length)
  for (const root of trees) {
    place(root, 0)
    slot += 0.35
  }
  if (singles.length) {
    const cols = Math.max(1, Math.ceil(Math.sqrt(singles.length * (trees.length ? 0.6 : 1.4))))
    const base = slot
    singles.forEach((id, index) => {
      local.set(id, {
        b: base + (index % cols),
        d: Math.floor(index / cols) * (GRID_LEVEL / LEVEL)
      })
    })
    slot = base + Math.min(cols, singles.length)
  }
  // Anything left (cycles) falls back to the grid tail.
  for (const id of cluster.members) {
    if (!local.has(id)) {
      local.set(id, { b: slot, d: 0 })
      slot += 1
    }
  }
  let minB = Infinity
  let maxB = -Infinity
  let maxD = 0
  for (const pos of local.values()) {
    minB = Math.min(minB, pos.b)
    maxB = Math.max(maxB, pos.b)
    maxD = Math.max(maxD, pos.d)
  }
  const mid = (minB + maxB) / 2
  for (const pos of local.values()) {
    pos.b = (pos.b - mid) * SLOT
    pos.d = pos.d * LEVEL
  }
  return {
    local,
    roots,
    breadth: (maxB - minB) * SLOT + NODE + CLUSTER_PAD * 2,
    depth: maxD * LEVEL + NODE + CLUSTER_PAD * 2
  }
}

function orientation(theta) {
  const c = Math.cos(theta)
  const s = Math.sin(theta)
  if (Math.abs(c) >= Math.abs(s)) {
    return c >= 0 ? { u: [1, 0], v: [0, 1] } : { u: [-1, 0], v: [0, 1] }
  }
  return s >= 0 ? { u: [0, 1], v: [1, 0] } : { u: [0, -1], v: [1, 0] }
}

function placeCluster(shape, theta, radius) {
  const { u, v } = orientation(theta)
  const ax = Math.cos(theta) * radius
  const ay = Math.sin(theta) * radius
  const offset = CLUSTER_PAD + NODE / 2
  const cx = ax + u[0] * shape.depth / 2
  const cy0 = ay + u[1] * shape.depth / 2
  const w = u[0] ? shape.depth : shape.breadth
  const h = u[0] ? shape.breadth : shape.depth
  return {
    positions() {
      const out = new Map()
      for (const [id, pos] of shape.local) {
        out.set(id, {
          x: ax + v[0] * pos.b + u[0] * (pos.d + offset),
          y: ay + v[1] * pos.b + u[1] * (pos.d + offset)
        })
      }
      return out
    },
    rect: { x1: cx - w / 2, x2: cx + w / 2, y1: cy0 - h / 2, y2: cy0 + h / 2 }
  }
}

function overlaps(a, b, margin) {
  return a.x1 < b.x2 + margin && b.x1 < a.x2 + margin && a.y1 < b.y2 + margin && b.y1 < a.y2 + margin
}

/** Liang–Barsky clip test: does segment p→q pass through the rect (grown by margin)? */
function segmentHitsRect(p, q, rect, margin) {
  const x1 = rect.x1 - margin
  const x2 = rect.x2 + margin
  const y1 = rect.y1 - margin
  const y2 = rect.y2 + margin
  const dx = q.x - p.x
  const dy = q.y - p.y
  let t0 = 0
  let t1 = 1
  const checks = [[-dx, p.x - x1], [dx, x2 - p.x], [-dy, p.y - y1], [dy, y2 - p.y]]
  for (const [pk, qk] of checks) {
    if (pk === 0) {
      if (qk < 0) return false
      continue
    }
    const r = qk / pk
    if (pk < 0) {
      if (r > t1) return false
      if (r > t0) t0 = r
    } else {
      if (r < t0) return false
      if (r < t1) t1 = r
    }
  }
  return t0 <= t1
}

function rectCenter(rect) {
  return { x: (rect.x1 + rect.x2) / 2, y: (rect.y1 + rect.y2) / 2 }
}

/** -1 when the rect collides; otherwise how many trunk lines would cross a cluster. */
function crossingsAt(result, placed, trunks) {
  const origin = { x: 0, y: 0 }
  const center = rectCenter(result.rect)
  let crossings = 0
  for (const box of placed) {
    if (overlaps(box, result.rect, CLUSTER_GAP)) return -1
    if (!box.gateway && segmentHitsRect(origin, center, box, 6)) crossings += 1
  }
  for (const target of trunks) {
    if (segmentHitsRect(origin, target, result.rect, 6)) crossings += 1
  }
  return crossings
}

const ANGLE_PENALTY = 140
const CROSS_PENALTY = 420
const ANGLE_STEPS = [0]
for (let step = Math.PI / 36; step <= Math.PI + 1e-6; step += Math.PI / 36) ANGLE_STEPS.push(step, -step)

/** Alternates large and small clusters so no side of the ring gets all the heavy groups. */
function balanceOrder(items) {
  const bySize = [...items].sort((a, b) => b.shape.breadth * b.shape.depth - a.shape.breadth * a.shape.depth)
  const out = []
  while (bySize.length) {
    out.push(bySize.shift())
    if (bySize.length) out.push(bySize.pop())
  }
  return out
}

function computeLayout(model) {
  const shapes = balanceOrder(model.clusters.map((cluster) => ({ cluster, shape: layoutCluster(model, cluster) })))
  const footprint = shapes.reduce((sum, item) => sum + item.shape.breadth + CLUSTER_GAP, 0)
  const ring = Math.max(150, (footprint / (2 * Math.PI)) * 0.45)
  const positions = new Map([[GATEWAY_ID, { x: 0, y: 0 }]])
  const placed = [{ x1: -70, x2: 70, y1: -70, y2: 70, gateway: true }]
  const trunks = []
  let acc = 0
  for (const { shape } of shapes) {
    const share = shape.breadth + CLUSTER_GAP
    const theta = -Math.PI / 2 + ((acc + share / 2) / footprint) * Math.PI * 2
    acc += share
    let result = null
    let bestScore = Infinity
    for (const angle of ANGLE_STEPS) {
      const drift = Math.abs(Math.atan2(Math.sin(angle), Math.cos(angle)))
      for (let radius = ring; radius < ring + 3000; radius += 16) {
        const base = radius + drift * ANGLE_PENALTY
        if (base >= bestScore) break
        const candidate = placeCluster(shape, theta + angle, radius)
        const crossings = crossingsAt(candidate, placed, trunks)
        if (crossings < 0) continue
        const score = base + crossings * CROSS_PENALTY
        if (score < bestScore) {
          result = candidate
          bestScore = score
        }
        if (!crossings) break
      }
    }
    result ||= placeCluster(shape, theta, ring + 3000)
    placed.push(result.rect)
    trunks.push(rectCenter(result.rect))
    for (const [id, pos] of result.positions()) positions.set(id, pos)
  }
  return { positions, shapes }
}

function buildElements(model, layout) {
  const elements = []
  const counts = new Map()
  for (const { cluster, shape } of layout.shapes) {
    const [stroke, fill] = cluster.color
    const assets = cluster.members.filter((id) => model.nodes.get(id)?.kind === 'asset').length
    elements.push({
      group: 'nodes',
      data: {
        id: cluster.key,
        kind: 'group',
        name: cluster.label,
        label: t('home.topologyGroupCount', { name: cluster.label, n: assets }),
        stroke,
        fill
      },
      classes: 'group-node'
    })
    counts.set(cluster.key, shape.roots.filter((id) => {
      const upstream = model.upstreamOf.get(id)
      return !upstream || upstream === GATEWAY_ID
    }).length)
  }
  for (const [id, node] of model.nodes) {
    const abnormal = abnormalById.value.get(id)
    const classes = [node.kind || 'asset']
    if (node.kind === 'asset') {
      classes.push(healthClass(node))
      if (abnormal) classes.push('has-alert')
    }
    const cluster = model.clusterOf.get(id)
    elements.push({
      group: 'nodes',
      data: {
        ...node,
        id,
        label: nodeLabel(node),
        parent: node.kind === 'gateway' ? undefined : cluster,
        stroke: cluster ? model.clusters.find((item) => item.key === cluster)?.color?.[0] : undefined
      },
      position: layout.positions.get(id) || { x: 0, y: 0 },
      classes: classes.join(' ')
    })
  }
  for (const [key, count] of counts) {
    if (!count || !model.nodes.has(GATEWAY_ID)) continue
    elements.push({
      group: 'edges',
      data: { id: `trunk:${key}`, source: GATEWAY_ID, target: key, count },
      classes: 'trunk'
    })
  }
  for (const [downstream, upstream] of model.upstreamOf) {
    if (upstream === GATEWAY_ID) continue
    if (!model.nodes.has(downstream) || !model.nodes.has(upstream)) continue
    elements.push({
      group: 'edges',
      data: { id: `edge:${upstream}:${downstream}`, source: upstream, target: downstream },
      classes: model.clusterOf.get(upstream) === model.clusterOf.get(downstream) ? 'link' : 'link cross'
    })
  }
  return elements
}

function styles() {
  return [
    {
      selector: 'node',
      style: {
        width: NODE,
        height: NODE,
        shape: 'round-rectangle',
        'corner-radius': 7,
        'background-color': '#ffffff',
        'border-width': 2,
        'border-color': '#cbd5e1',
        label: 'data(label)',
        color: '#1b2430',
        'font-family': 'IBM Plex Sans, Microsoft YaHei, sans-serif',
        'font-size': 11,
        'font-weight': 500,
        'text-wrap': 'wrap',
        'text-max-width': NODE - 6,
        'text-valign': 'center',
        'text-halign': 'center',
        'line-height': 1.25,
        'min-zoomed-font-size': 7,
        'overlay-opacity': 0,
        'transition-property': 'opacity',
        'transition-duration': 120
      }
    },
    {
      selector: 'node.normal',
      style: { 'border-color': '#4d9b72', 'background-color': '#f4fbf7' }
    },
    {
      selector: 'node.abnormal',
      style: { 'border-color': '#d97706', 'border-width': 3, 'background-color': '#fff7e8' }
    },
    {
      selector: 'node.offline',
      style: { 'border-color': '#dc2626', 'border-width': 3, 'background-color': '#fff1f1', color: '#7f1d1d' }
    },
    {
      selector: 'node.has-alert',
      style: {
        'shadow-color': '#dc2626',
        'shadow-blur': 14,
        'shadow-opacity': 0.3,
        'shadow-offset-x': 0,
        'shadow-offset-y': 0
      }
    },
    {
      selector: 'node.gateway',
      style: {
        width: 84,
        height: 84,
        shape: 'ellipse',
        'background-color': '#2f5d9f',
        'border-color': '#1e4479',
        'border-width': 4,
        color: '#ffffff',
        'font-size': 13,
        'font-weight': 700,
        'shadow-color': '#2f5d9f',
        'shadow-blur': 22,
        'shadow-opacity': 0.35,
        'shadow-offset-x': 0,
        'shadow-offset-y': 0
      }
    },
    {
      selector: 'node.hidden',
      style: {
        'background-color': '#fff7ed',
        'border-color': '#b54708',
        'border-style': 'dashed',
        color: '#7c2d12',
        'font-size': 10
      }
    },
    {
      selector: 'node.group-node',
      style: {
        shape: 'round-rectangle',
        'corner-radius': 14,
        'background-color': 'data(fill)',
        'background-opacity': 0.72,
        'border-color': 'data(stroke)',
        'border-width': 1.5,
        'border-opacity': 0.55,
        label: 'data(label)',
        color: 'data(stroke)',
        'font-size': 12,
        'font-weight': 700,
        'text-valign': 'top',
        'text-halign': 'center',
        'text-margin-y': -6,
        'text-wrap': 'none',
        'text-max-width': 9999,
        'min-zoomed-font-size': 5,
        padding: CLUSTER_PAD - 18,
        'compound-sizing-wrt-labels': 'exclude'
      }
    },
    {
      selector: 'edge',
      style: {
        width: 1.6,
        'curve-style': 'straight',
        'line-color': '#93a4b8',
        'source-arrow-shape': 'triangle',
        'source-arrow-color': '#7a8da3',
        'arrow-scale': 0.7,
        opacity: 0.9,
        'overlay-opacity': 0,
        'transition-property': 'opacity, line-color',
        'transition-duration': 120
      }
    },
    {
      selector: 'edge.cross',
      style: { 'line-style': 'dashed', 'line-color': '#b54708', 'source-arrow-color': '#b54708' }
    },
    {
      selector: 'edge.trunk',
      style: {
        width: 4,
        'line-color': '#7d95b4',
        'source-arrow-shape': 'none',
        opacity: 0.55,
        'z-compound-depth': 'bottom'
      }
    },
    {
      selector: 'node:selected',
      style: {
        'border-color': '#2563eb',
        'border-width': 4,
        'shadow-color': '#2563eb',
        'shadow-blur': 18,
        'shadow-opacity': 0.3
      }
    },
    {
      selector: '.faded',
      style: { opacity: 0.16 }
    },
    {
      selector: 'edge.path',
      style: { 'line-color': '#2563eb', 'source-arrow-color': '#2563eb', width: 2.6, opacity: 1 }
    },
    {
      selector: 'edge.trunk.path',
      style: { width: 5, opacity: 0.9 }
    },
    {
      selector: '.search-dim',
      style: { opacity: 0.14 }
    },
    {
      selector: 'edge.search-dim',
      style: { opacity: 0.06 }
    }
  ]
}

/** Upstream chain to the gateway plus the downstream subtree of a node. */
function chainOf(node) {
  let chain = node.union(node.successors()).union(node.predecessors())
  chain.nodes('[kind != "group"]').forEach((item) => {
    const parent = item.parent()
    if (parent.nonempty()) chain = chain.union(parent)
  })
  const roots = chain.nodes('[kind != "group"]').filter((item) =>
    item.incomers('edge.link').empty() && item.data('kind') !== 'gateway')
  roots.forEach((root) => {
    const parent = root.parent()
    if (parent.nonempty()) {
      chain = chain.union(parent.incomers('edge.trunk')).union(cy.getElementById(GATEWAY_ID))
    }
  })
  return chain
}

function highlight(node) {
  if (!cy) return
  const chain = chainOf(node)
  cy.batch(() => {
    cy.elements().addClass('faded')
    chain.removeClass('faded')
    chain.edges().addClass('path')
  })
}

function clearHighlight() {
  cy?.batch(() => {
    cy.elements().removeClass('faded path')
  })
}

function showTip(node) {
  if (!canvasRef.value) return
  const pos = node.renderedPosition()
  const half = node.renderedOuterHeight() / 2
  const data = node.data()
  hover.value = {
    name: data.kind === 'gateway' ? 'Gateway' : (data.name || t('assets.topologyHidden')),
    ip: data.privateIp || '',
    group: data.groupName || '',
    x: pos.x,
    y: pos.y - half - 8
  }
}

async function createGraph() {
  if (!canvasRef.value) return
  destroy()
  await nextTick()
  const model = buildModel()
  const layout = computeLayout(model)
  cy = cytoscape({
    container: canvasRef.value,
    elements: buildElements(model, layout),
    style: styles(),
    layout: { name: 'preset' },
    minZoom: 0.08,
    maxZoom: 3,
    wheelSensitivity: 8,
    boxSelectionEnabled: false,
    autoungrabify: true,
    textureOnViewport: assetNodes.value.length > 150,
    pixelRatio: 1
  })
  cy.on('tap', 'node[kind = "asset"]', (event) => {
    selectedId.value = String(event.target.id())
  })
  cy.on('tap', (event) => {
    if (event.target === cy) clearSelection()
  })
  cy.on('mouseover', 'node[kind != "group"]', (event) => {
    highlight(event.target)
    showTip(event.target)
  })
  cy.on('mouseout', 'node[kind != "group"]', () => {
    hover.value = null
    clearHighlight()
    applySearch()
  })
  cy.on('viewport', () => {
    hover.value = null
  })
  frameReadable()
  applySearch()
  emit('ready')
}

function runLayout() {
  if (!cy) return
  const model = buildModel()
  const layout = computeLayout(model)
  cy.batch(() => {
    layout.positions.forEach((pos, id) => cy.getElementById(id).position(pos))
  })
  frameReadable()
}

/** Fits into the area not covered by the floating KPI strip / incident panel. */
function viewportFor(minZoom = 0) {
  const width = cy.width()
  const height = cy.height()
  const compact = width < 900
  const inset = compact ? { top: 140, right: 16, bottom: 56, left: 16 } : props.insets
  const box = cy.elements().boundingBox()
  const availW = Math.max(120, width - inset.left - inset.right)
  const availH = Math.max(120, height - inset.top - inset.bottom)
  let zoom = Math.min(availW / Math.max(box.w, 1), availH / Math.max(box.h, 1), 1.4)
  zoom = Math.max(zoom, minZoom)
  const gateway = cy.getElementById(GATEWAY_ID)
  const anchor = zoom > Math.min(availW / Math.max(box.w, 1), availH / Math.max(box.h, 1)) && gateway.nonempty()
    ? gateway.position()
    : { x: (box.x1 + box.x2) / 2, y: (box.y1 + box.y2) / 2 }
  return {
    zoom,
    pan: {
      x: inset.left + availW / 2 - anchor.x * zoom,
      y: inset.top + availH / 2 - anchor.y * zoom
    }
  }
}

function frameReadable() {
  if (!cy) return
  cy.viewport(viewportFor(0.36))
}

function fit() {
  if (!cy) return
  cy.animate({ ...viewportFor(), duration: 260 })
}

function applySearch() {
  if (!cy) return
  const needle = query.value.trim().toLowerCase()
  cy.batch(() => {
    cy.elements().removeClass('search-dim')
    if (!needle) return
    const matches = cy.nodes('[kind = "asset"]').filter((node) =>
      [node.data('name'), node.data('privateIp'), node.data('groupName')]
        .some((value) => String(value || '').toLowerCase().includes(needle)))
    cy.elements().addClass('search-dim')
    matches.removeClass('search-dim')
    matches.connectedEdges().removeClass('search-dim')
    matches.parent().removeClass('search-dim')
  })
}

function focusFirstMatch() {
  if (!cy || !query.value.trim()) return
  const node = cy.nodes('[kind = "asset"]').filter((item) => !item.hasClass('search-dim')).first()
  if (node?.length) focusAsset(node.id())
}

function focusAsset(assetId) {
  if (!cy || !assetId) return
  const node = cy.getElementById(String(assetId))
  if (!node.length) return
  selectedId.value = String(assetId)
  cy.$(':selected').unselect()
  node.select()
  cy.animate({
    center: { eles: node },
    zoom: Math.max(cy.zoom(), 1),
    duration: 300
  })
}

function clearSelection() {
  selectedId.value = ''
  cy?.$(':selected').unselect()
}

function destroy() {
  hover.value = null
  cy?.destroy()
  cy = null
}

watch(
  () => [props.topology, props.groups, props.abnormalAssets],
  () => createGraph(),
  { deep: true }
)

watch(query, applySearch)

defineExpose({ fit, focusAsset })

if (typeof ResizeObserver !== 'undefined') {
  resizeObserver = new ResizeObserver(() => {
    cy?.resize()
  })
}

watch(canvasRef, (element) => {
  resizeObserver?.disconnect()
  if (element) {
    resizeObserver?.observe(element)
    createGraph()
  }
}, { flush: 'post' })

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  destroy()
})
</script>

<style scoped>
.network-topology {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 520px;
  overflow: hidden;
  background:
    radial-gradient(circle at 46% 52%, rgba(47, 93, 159, 0.09), transparent 42%),
    linear-gradient(rgba(113, 134, 158, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(113, 134, 158, 0.06) 1px, transparent 1px),
    #f7f9fc;
  background-size: auto, 28px 28px, 28px 28px, auto;
}

.topology-canvas {
  position: absolute;
  inset: 0;
  cursor: grab;
}
.topology-canvas:active { cursor: grabbing; }

.topology-controls {
  position: absolute;
  z-index: 3;
  top: 100px;
  left: 18px;
  display: flex;
  align-items: center;
  gap: 6px;
  width: min(360px, calc(100% - 36px));
  padding: 6px;
  border: 1px solid rgba(197, 206, 217, 0.9);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 8px 24px rgba(27, 36, 48, 0.09);
  backdrop-filter: blur(10px);
}

.control-btn {
  appearance: none;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--ops-text-secondary);
  cursor: pointer;
}
.control-btn:hover { background: var(--ops-color-primary-soft); color: var(--ops-color-primary); }

.topology-legend {
  position: absolute;
  z-index: 3;
  left: 18px;
  bottom: 18px;
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  padding: 7px 10px;
  border: 1px solid rgba(197, 206, 217, 0.8);
  border-radius: 7px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: var(--ops-shadow-sm);
  color: var(--ops-text-secondary);
  font-size: 11px;
  backdrop-filter: blur(8px);
}
.topology-legend span { display: inline-flex; align-items: center; gap: 5px; }
.dot, .status-dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; flex-shrink: 0; }
.trunk { width: 18px; height: 4px; border-radius: 2px; background: #7d95b4; opacity: 0.7; display: inline-block; }
.normal { background: #31875b; }
.abnormal { background: #d97706; }
.offline { background: #dc2626; }

.node-tip {
  position: absolute;
  z-index: 5;
  transform: translate(-50%, -100%);
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 280px;
  padding: 6px 9px;
  border-radius: 6px;
  background: rgba(27, 36, 48, 0.92);
  color: #fff;
  font-size: 11px;
  line-height: 1.4;
  pointer-events: none;
  white-space: nowrap;
}
.node-tip strong { font-size: 12px; overflow: hidden; text-overflow: ellipsis; }
.node-tip span { color: rgba(255, 255, 255, 0.72); }

.node-popover {
  position: absolute;
  z-index: 4;
  left: 18px;
  bottom: 58px;
  width: 260px;
  padding: 13px 14px 10px;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 14px 36px rgba(27, 36, 48, 0.14);
  backdrop-filter: blur(12px);
}
.popover-close {
  position: absolute;
  top: 5px;
  right: 7px;
  border: 0;
  background: transparent;
  color: var(--ops-text-muted);
  font-size: 18px;
  cursor: pointer;
}
.node-title { display: flex; align-items: center; gap: 7px; padding-right: 18px; }
.node-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.node-meta { margin-top: 5px; color: var(--ops-text-secondary); font-size: 11px; }
.node-alert { margin-top: 7px; color: var(--ops-danger); font-size: 12px; font-weight: 500; }
.node-actions { display: flex; justify-content: flex-end; margin-top: 5px; }

.topology-empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: var(--ops-text-muted);
  font-size: 13px;
  pointer-events: none;
}
</style>
