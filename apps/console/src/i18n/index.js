import { createI18n } from 'vue-i18n'
import en from './en'
import zh from './zh'

/** Browser language → app locale; default English. Only `en` / `zh`. */
export function detectLocale() {
  const raw = String(navigator.language || navigator.userLanguage || '').toLowerCase()
  if (raw.startsWith('zh')) return 'zh'
  return 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: 'en',
  messages: { en, zh }
})

/** Use from plain JS modules (outside setup()). */
export function t(key, ...args) {
  return i18n.global.t(key, ...args)
}

export default i18n
