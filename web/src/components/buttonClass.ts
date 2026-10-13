import styles from './ui.module.css';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

export function buttonClass(variant: ButtonVariant = 'primary', size: 'small' | 'normal' = 'normal', block = false): string {
  return [styles.button, styles[variant], size === 'small' && styles.small, block && styles.block].filter(Boolean).join(' ');
}
