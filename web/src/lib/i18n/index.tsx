// i18n: the language registry, the provider and t().
// Adding a language: create i18n/<code>.ts exporting a `Dict` (keys mirror en.ts,
// missing keys fall back to English) and add one line to LANGUAGES below.
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { en, type Dict } from './en'
import { ru } from './ru'
import { ja } from './ja'
import { zh } from './zh'
import { setIntlLocale } from '../format'

export interface Language { code: string; name: string; native: string; dict: Dict }

export const LANGUAGES: Language[] = [
  { code: 'en', name: 'English', native: 'English', dict: en },
  { code: 'ru', name: 'Russian', native: 'Русский', dict: ru },
  { code: 'ja', name: 'Japanese', native: '日本語', dict: ja },
  { code: 'zh', name: 'Chinese', native: '中文', dict: zh },
]
export const FALLBACK = 'en'
export const AUTO = 'auto'
const KEY = 'zan-lang'

/** 'auto' or a language code → the actual code (browser language when auto). */
export function resolveLang(choice: string): string {
  if (choice !== AUTO && LANGUAGES.some(l => l.code === choice)) return choice
  const nav = (navigator.language || 'en').slice(0, 2).toLowerCase()
  return LANGUAGES.some(l => l.code === nav) ? nav : FALLBACK
}

interface I18nState {
  /** The stored choice: 'auto' or a language code. */
  choice: string
  /** The resolved language code. */
  lang: string
  setLang: (choice: string) => void
  t: (key: string, vars?: Record<string, string | number>) => string
}
const Ctx = createContext<I18nState>(null!)
export function useI18n() { return useContext(Ctx) }

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [choice, setChoice] = useState(() => localStorage.getItem(KEY) || AUTO)
  const lang = useMemo(() => resolveLang(choice), [choice])
  useEffect(() => {
    document.documentElement.lang = lang
    setIntlLocale(lang)
  }, [lang])
  const setLang = useCallback((c: string) => {
    setChoice(c)
    if (c === AUTO) localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, c)
  }, [])
  const dict = useMemo(() => LANGUAGES.find(l => l.code === lang)?.dict || en, [lang])
  const t = useCallback((key: string, vars?: Record<string, string | number>) => {
    let s = dict[key] ?? en[key] ?? key
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
    return s
  }, [dict])
  return <Ctx.Provider value={{ choice, lang, setLang, t }}>{children}</Ctx.Provider>
}
