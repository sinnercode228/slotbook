import { LANGS } from '../i18n/dictionary';
import { useI18n } from '../i18n/useI18n';
import styles from './LanguageSwitch.module.css';

export function LanguageSwitch() {
  const { lang, setLang, t } = useI18n();
  return (
    <div className={styles.switch} role="group" aria-label={t('app.language')}>
      {LANGS.map((code) => (
        <button
          key={code}
          type="button"
          className={styles.option}
          aria-pressed={code === lang}
          lang={code}
          onClick={() => setLang(code)}
        >
          {code.toUpperCase()}
        </button>
      ))}
    </div>
  );
}
