import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import type { BookingApi } from '../api/client';
import { ApiContext } from '../app/ApiContext';
import type { Lang } from '../i18n/dictionary';
import { I18nProvider } from '../i18n/I18nProvider';

/** A BookingApi whose unlisted methods fail loudly if a component calls them. */
export function fakeApi(overrides: Partial<BookingApi>): BookingApi {
  const missing = (name: string) => () => {
    throw new Error(`unexpected call to ${name}`);
  };
  return new Proxy(overrides as BookingApi, {
    get(target, prop: string) {
      if (prop in target) return target[prop as keyof BookingApi];
      if (prop === 'mode') return 'http';
      if (prop === 'subscribe') return () => () => {};
      return missing(prop);
    },
  });
}

export function renderWith(ui: ReactElement, { api = fakeApi({}), lang = 'en' }: { api?: BookingApi; lang?: Lang } = {}) {
  return render(
    <ApiContext.Provider value={api}>
      <I18nProvider lang={lang}>{ui}</I18nProvider>
    </ApiContext.Provider>,
  );
}
