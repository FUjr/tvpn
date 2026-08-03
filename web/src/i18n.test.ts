import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { hasTranslation, missingTranslations, setLocale } from './i18n'

describe('translations', () => {
  it('keeps Chinese and English resources in sync', () => {
    expect(missingTranslations()).toEqual([])
  })

  it('defines every literal message id used by the application', () => {
    const source = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
    const ids = [...source.matchAll(/\bt\('([^']+)'/g)].map(match => match[1])
    for (const locale of ['zh-CN', 'en-US'] as const) {
      setLocale(locale)
      for (const id of ids) expect(hasTranslation(id), `${locale}: ${id}`).toBe(true)
    }
  })
})
