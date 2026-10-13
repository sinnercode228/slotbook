import { ChevronLeft } from '../components/icons';
import { useI18n } from '../i18n/useI18n';
import styles from './Widget.module.css';

export const TOTAL_STEPS = 5;

export function StepHeader({ step, title, onBack }: { step: number; title: string; onBack?: () => void }) {
  const { t } = useI18n();
  return (
    <div className={styles.stepHeader}>
      {onBack && (
        <button type="button" className={styles.iconButton} onClick={onBack} aria-label={t('common.back')}>
          <ChevronLeft />
        </button>
      )}
      <div className={styles.stepText}>
        <span className={styles.progress}>{t('step.progress', { current: step, total: TOTAL_STEPS })}</span>
        <h2 className={styles.title}>{title}</h2>
      </div>
    </div>
  );
}
