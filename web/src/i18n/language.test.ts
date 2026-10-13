import { describe, expect, it } from 'vitest';
import { dictionaries } from './dictionary';
import { detectLanguage, interpolate } from './language';

describe('detectLanguage', () => {
  it('prefers ?lang= in the query string, then in the hash route', () => {
    expect(detectLanguage({ search: '?lang=ru', hash: '#/', stored: 'en', browser: ['en-US'] })).toBe('ru');
    expect(detectLanguage({ search: '', hash: '#/admin?lang=ru', stored: null, browser: ['en-US'] })).toBe('ru');
  });

  it('falls back to the stored choice, then the browser languages, then English', () => {
    expect(detectLanguage({ search: '?lang=de', hash: '', stored: 'ru', browser: ['en'] })).toBe('ru');
    expect(detectLanguage({ search: '', hash: '', stored: null, browser: ['ru-RU', 'en'] })).toBe('ru');
    expect(detectLanguage({ search: '', hash: '', stored: null, browser: ['de-DE'] })).toBe('en');
  });
});

describe('dictionaries', () => {
  it('use the same placeholders in every language', () => {
    const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    for (const [key, english] of Object.entries(dictionaries.en)) {
      expect(placeholders(dictionaries.ru[key as keyof typeof dictionaries.en]), key).toEqual(placeholders(english));
    }
  });
});

describe('interpolate', () => {
  it('fills placeholders and leaves unknown ones visible', () => {
    expect(interpolate('Step {current} of {total}', { current: 2, total: 5 })).toBe('Step 2 of 5');
    expect(interpolate('Hello {name}', {})).toBe('Hello {name}');
  });
});
