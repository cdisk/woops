import i18n from '../../i18n'

/**
 * Display name for a metric, preferring the current UI language.
 *
 * `/api/monitor/items` returns `name` straight from `monitor_item_defs`, which is
 * seed data written once at first startup — so it is stuck in whatever language
 * that deployment was seeded with and cannot follow the browser language. For the
 * built-in metrics we therefore look the name up in `monitorItems.*` instead, and
 * only fall back to the API value for metrics the i18n catalog does not know
 * (anything a user added, where their own wording is the right answer).
 *
 * @param {string} itemId e.g. `cpu.usage_percent`
 * @param {string} [apiName] the `name` the API returned, used as fallback
 */
export function monitorItemLabel(itemId, apiName) {
  if (!itemId) return apiName || ''
  // Dots would be read as a nested key path, so the catalog uses underscores.
  const key = `monitorItems.${String(itemId).replace(/\./g, '_')}`
  const { te, t } = i18n.global
  return te(key) ? t(key) : apiName || itemId
}
