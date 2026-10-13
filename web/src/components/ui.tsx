import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { useI18n } from '../i18n/useI18n';
import { buttonClass, type ButtonVariant } from './buttonClass';
import { AlertIcon, Check } from './icons';
import styles from './ui.module.css';
import { useErrorMessage } from './useErrorMessage';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: 'small' | 'normal';
  block?: boolean;
  busy?: boolean;
}

export function Button({ variant, size, block, busy = false, className, children, disabled, type = 'button', ...rest }: ButtonProps) {
  return (
    <button
      type={type}
      className={[buttonClass(variant, size, block), className].filter(Boolean).join(' ')}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      {...rest}
    >
      {busy && <span className={styles.spinner} aria-hidden="true" />}
      {children}
    </button>
  );
}

export function Spinner({ label }: { label: string }) {
  return (
    <span role="status" className={styles.state}>
      <span className={styles.spinner} aria-hidden="true" />
      <span className="visually-hidden">{label}</span>
    </span>
  );
}

type Tone = 'info' | 'warning' | 'danger' | 'success';

export function Notice({
  tone = 'info',
  title,
  children,
  action,
}: {
  tone?: Tone;
  title?: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
}) {
  const toneClass = { info: styles.info, warning: styles.warning, danger: styles.dangerNotice, success: styles.success }[tone];
  return (
    <div className={`${styles.notice} ${toneClass}`} role={tone === 'danger' || tone === 'warning' ? 'alert' : 'status'}>
      {tone === 'success' ? <Check /> : <AlertIcon />}
      <div className={styles.noticeBody}>
        {title && <p className={styles.noticeTitle}>{title}</p>}
        {children && <div>{children}</div>}
        {action && <div className={styles.noticeAction}>{action}</div>}
      </div>
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useI18n();
  const message = useErrorMessage();
  return (
    <Notice
      tone="danger"
      action={
        onRetry && (
          <Button variant="secondary" size="small" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        )
      }
    >
      {message(error)}
    </Notice>
  );
}

export function SkeletonList({ rows = 3, label }: { rows?: number; label: string }) {
  return (
    <div className={styles.skeleton} role="status" aria-label={label}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className={styles.skeletonRow} />
      ))}
    </div>
  );
}

export function Badge({ tone, children }: { tone?: 'accent' | 'warning' | 'danger' | 'info'; children: ReactNode }) {
  return (
    <span className={styles.badge} data-tone={tone}>
      {children}
    </span>
  );
}
