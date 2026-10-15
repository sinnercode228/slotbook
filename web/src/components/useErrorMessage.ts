import { ApiError } from '../api/errors';
import { useI18n } from '../i18n/useI18n';

/** A plain-language message for a failed request. */
export function useErrorMessage(): (error: unknown) => string {
  const { t } = useI18n();
  return (error) => {
    const apiError = error instanceof ApiError ? error : undefined;
    if (apiError?.code === 'network') return t('common.error.network');
    if (apiError?.code === 'rate_limited') {
      const seconds = apiError.retryAfter;
      if (!seconds) return t('common.error.rate_limited');
      const wait = seconds < 60 ? t('common.seconds', { n: seconds }) : t('common.minutes', { n: Math.ceil(seconds / 60) });
      return t('common.error.rate_limited_wait', { wait });
    }
    return t('common.error.generic');
  };
}
