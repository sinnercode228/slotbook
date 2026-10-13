import { createContext, useContext } from 'react';
import type { Lang, MessageKey } from './dictionary';

export type Translate = (key: MessageKey, values?: Record<string, string | number>) => string;

export interface I18n {
  lang: Lang;
  /** BCP 47 locale for Intl formatting. */
  locale: string;
  setLang: (lang: Lang) => void;
  t: Translate;
}

export const I18nContext = createContext<I18n | null>(null);

export function useI18n(): I18n {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n needs an I18nProvider');
  return ctx;
}
