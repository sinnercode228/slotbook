import { LANGS, type Lang } from './dictionary';

const isLang = (value: string | null | undefined): value is Lang => LANGS.includes(value as Lang);

export interface LanguageSources {
  search: string;
  hash: string;
  stored: string | null;
  browser: readonly string[];
}

/** ?lang= (in the query or in the hash route) wins, then the saved choice, then the browser. */
export function detectLanguage({ search, hash, stored, browser }: LanguageSources): Lang {
  const hashQuery = hash.includes('?') ? hash.slice(hash.indexOf('?')) : '';
  for (const candidate of [new URLSearchParams(search).get('lang'), new URLSearchParams(hashQuery).get('lang'), stored]) {
    if (isLang(candidate)) return candidate;
  }
  for (const tag of browser) {
    const base = tag.toLowerCase().split('-')[0];
    if (isLang(base)) return base;
  }
  return 'en';
}

export function interpolate(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (match, name: string) => (name in values ? String(values[name]) : match));
}
