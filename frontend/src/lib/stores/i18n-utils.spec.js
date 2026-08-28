import { describe, expect, it } from 'vitest';
import { getPluralCategory, negotiateLocale } from './i18n-utils.js';

const supportedLocales = [
  { code: 'en' },
  { code: 'de' },
  { code: 'es' },
  { code: 'ar' },
  { code: 'pt-BR' },
  { code: 'ru' },
  { code: 'zh-CN' },
];

describe('negotiateLocale', () => {
  it.each([
    ['pt-BR', 'pt-BR'],
    ['pt-br', 'pt-BR'],
    ['pt_BR', 'pt-BR'],
    ['zh-CN', 'zh-CN'],
    ['zh_cn', 'zh-CN'],
    ['de-CH', 'de'],
    ['ru', 'ru'],
    ['ru-RU', 'ru'],
    ['ru_RU', 'ru'],
    ['fr-FR', 'en'],
  ])('maps %s to %s', (browserLocale, expected) => {
    expect(negotiateLocale(browserLocale, supportedLocales, 'en')).toBe(expected);
  });
});

describe('getPluralCategory', () => {
  it.each([
    [0, 'zero'],
    [1, 'one'],
    [2, 'two'],
    [3, 'few'],
    [11, 'many'],
    [100, 'other'],
  ])('selects the Arabic category for %d', (count, expected) => {
    expect(getPluralCategory('ar', count)).toBe(expected);
  });

  it.each([
    [0, 'many'],
    [1, 'one'],
    [2, 'few'],
    [5, 'many'],
    [11, 'many'],
    [21, 'one'],
    [22, 'few'],
    [25, 'many'],
  ])('selects the Russian category for %d', (count, expected) => {
    expect(getPluralCategory('ru', count)).toBe(expected);
  });

  it('uses the single Simplified Chinese category', () => {
    expect(getPluralCategory('zh-CN', 0)).toBe('other');
    expect(getPluralCategory('zh-CN', 2)).toBe('other');
  });
});
