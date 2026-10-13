import { ApiError } from '../api/errors';
import { useI18n } from '../i18n/useI18n';

/** A plain-language message for a failed request. */
export function useErrorMessage(): (error: unknown) => string {
  const { t } = useI18n();
  return (error) =>
    error instanceof ApiError && error.code === 'network' ? t('common.error.network') : t('common.error.generic');
}
