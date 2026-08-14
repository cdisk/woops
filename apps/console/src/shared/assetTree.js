/**
 * Build el-tree-select data: groups (disabled) + assets (selectable by id).
 * @param {Array} groups server group tree
 * @param {Array} assets flat asset list
 * @param {{ ungroupedLabel?: string, showOnline?: boolean, onlineLabel?: string, offlineLabel?: string }} opts
 */
export function buildAssetSelectTree(groups, assets, opts = {}) {
  const {
    ungroupedLabel = 'Ungrouped',
    showOnline = false,
    onlineLabel = 'Online',
    offlineLabel = 'Offline'
  } = opts

  const byGroup = new Map()
  for (const a of assets || []) {
    const gid = a.groupId || '__ungrouped__'
    if (!byGroup.has(gid)) byGroup.set(gid, [])
    byGroup.get(gid).push(a)
  }

  function assetNode(a) {
    const name = a.displayName || a.hostname || a.id
    const label = showOnline
      ? `${name} (${a.online ? onlineLabel : offlineLabel})`
      : name
    return {
      value: a.id,
      label,
      kind: 'asset',
      disabled: false,
      hostname: a.hostname || '',
      displayName: a.displayName || '',
      publicIp: a.publicIp || '',
      privateIp: a.privateIp || '',
      online: !!a.online
    }
  }

  function mapGroup(n) {
    const childGroups = (n.children || []).map(mapGroup)
    const assetKids = (byGroup.get(n.id) || []).map(assetNode)
    byGroup.delete(n.id)
    return {
      value: `group:${n.id}`,
      label: n.name,
      kind: 'group',
      disabled: true,
      children: [...childGroups, ...assetKids]
    }
  }

  const roots = (groups || []).map(mapGroup)
  const leftover = []
  for (const list of byGroup.values()) leftover.push(...list)
  if (leftover.length) {
    roots.push({
      value: 'group:__ungrouped__',
      label: ungroupedLabel,
      kind: 'ungrouped',
      disabled: true,
      children: leftover.map(assetNode)
    })
  }
  return roots
}

/** Match tree-select filter query against asset fields (and group labels). */
export function matchAssetTreeNode(query, data) {
  const q = String(query || '').trim().toLowerCase()
  if (!q) return true
  if (data.kind === 'group' || data.kind === 'ungrouped') {
    return String(data.label || '').toLowerCase().includes(q)
  }
  return [data.label, data.displayName, data.hostname, data.publicIp, data.privateIp]
    .some((v) => String(v || '').toLowerCase().includes(q))
}
