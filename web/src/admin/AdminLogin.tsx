import { useId, useState, type FormEvent } from 'react';
import { useApi } from '../app/ApiContext';
import { errorCode } from '../api/errors';
import { Button, Notice } from '../components/ui';
import { useErrorMessage } from '../components/useErrorMessage';
import { useI18n } from '../i18n/useI18n';
import styles from './Admin.module.css';

/** Asks for the admin bearer token and checks it with one cheap request. */
export function AdminLogin({ onSignedIn }: { onSignedIn: (token: string) => void }) {
  const api = useApi();
  const { t } = useI18n();
  const errorMessage = useErrorMessage();
  const id = useId();
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const value = token.trim();
    if (!value) return;
    setBusy(true);
    setError(null);
    try {
      await api.admin(value).listNotifications({ limit: 1 });
      onSignedIn(value);
    } catch (err) {
      setError(errorCode(err) === 'unauthorized' ? t('admin.login.invalid') : errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <form className={styles.login} onSubmit={submit}>
      <h1 className={styles.loginTitle}>{t('admin.login.title')}</h1>
      <p className={styles.muted}>{t('admin.login.lead')}</p>
      {api.mode === 'mock' && <Notice>{t('admin.login.demoHint')}</Notice>}
      <div className={styles.field}>
        <label className={styles.label} htmlFor={`${id}-token`}>
          {t('admin.login.token')}
        </label>
        <input
          id={`${id}-token`}
          className={styles.input}
          type="password"
          autoComplete="current-password"
          required
          value={token}
          aria-invalid={error !== null}
          onChange={(e) => setToken(e.target.value)}
        />
      </div>
      {error && <Notice tone="danger">{error}</Notice>}
      <Button type="submit" busy={busy} block>
        {t('admin.login.submit')}
      </Button>
    </form>
  );
}
