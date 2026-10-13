import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { dictionaries, type Lang } from './dictionary';
import { detectLanguage, interpolate } from './language';
import { I18nContext, type I18n } from './useI18n';

const STORAGE_KEY = 'slotbook.lang';

const LOCALES: Record<Lang, string> = { en: 'en-GB', ru: 'ru-RU' };

function readStored(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function initialLanguage(): Lang {
  return detectLanguage({
    search: window.location.search,
    hash: window.location.hash,
    stored: readStored(),
    browser: navigator.languages ?? [navigator.language],
  });
}

export function I18nProvider({ children, lang: fixed }: { children: ReactNode; lang?: Lang }) {
  const [lang, setLangState] = useState<Lang>(() => fixed ?? initialLanguage());

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Private mode: the choice lasts for this page only.
    }
  }, []);

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo<I18n>(() => {
    const dict = dictionaries[lang];
    return {
      lang,
      locale: LOCALES[lang],
      setLang,
      t: (key, values) => (values ? interpolate(dict[key], values) : dict[key]),
    };
  }, [lang, setLang]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}
