import { ApiError } from '../api/errors';
import { useI18n } from '../i18n/useI18n';

/** A plain-language message for a failed request. */
export function useErrorMessage(): (error: unknown) => string {
  const { t } = useI18n();
  return (error) => {
    const code = error instanceof ApiError ? error.code : undefined;
    if (code === 'network') return t('common.error.network');
    if (code === 'rate_limited') return t('common.error.rate_limited');
    return t('common.error.generic');
  };
}
