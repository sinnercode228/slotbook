import { useId, useRef, useState, type FormEvent, type RefObject } from 'react';
import type { Customer } from '../api/types';
import { Button, Notice } from '../components/ui';
import { normalizeCustomer, type CustomerField, type CustomerProblem } from '../domain/customer';
import type { MessageKey } from '../i18n/dictionary';
import { useI18n } from '../i18n/useI18n';
import type { ContactValues } from './contact';
import styles from './Widget.module.css';

export interface ContactFormProps {
  values: ContactValues;
  onChange: (values: ContactValues) => void;
  onSubmit: (customer: Customer) => void;
  busy?: boolean;
  disabled?: boolean;
  /** A message about the whole form, such as a failed request. */
  formError?: string | null;
  /** A field the server rejected. */
  serverProblem?: { field: CustomerField; problem: CustomerProblem } | null;
}

const FIELDS: CustomerField[] = ['name', 'phone', 'email'];

const FIELD_SETUP: Record<CustomerField, { label: MessageKey; type: string; autoComplete: string; inputMode?: 'tel' | 'email' }> = {
  name: { label: 'form.name', type: 'text', autoComplete: 'name' },
  phone: { label: 'form.phone', type: 'tel', autoComplete: 'tel', inputMode: 'tel' },
  email: { label: 'form.email', type: 'email', autoComplete: 'email', inputMode: 'email' },
};

export function ContactForm({ values, onChange, onSubmit, busy = false, disabled = false, formError, serverProblem }: ContactFormProps) {
  const { t } = useI18n();
  const id = useId();
  const [touched, setTouched] = useState<Partial<Record<CustomerField, boolean>>>({});
  const [submitted, setSubmitted] = useState(false);
  const inputs: Record<CustomerField, RefObject<HTMLInputElement | null>> = {
    name: useRef<HTMLInputElement>(null),
    phone: useRef<HTMLInputElement>(null),
    email: useRef<HTMLInputElement>(null),
  };

  const result = normalizeCustomer(values);
  const problems: Partial<Record<CustomerField, CustomerProblem>> = {};
  if (!result.ok) {
    for (const p of result.problems) problems[p.field] ??= p.problem;
  }
  const shownProblem = (field: CustomerField): CustomerProblem | undefined =>
    ((submitted || touched[field]) && problems[field]) ||
    (serverProblem?.field === field ? serverProblem.problem : undefined);

  function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitted(true);
    if (!result.ok) {
      const first = FIELDS.find((f) => problems[f]);
      if (first) inputs[first].current?.focus();
      return;
    }
    onSubmit(result.customer);
  }

  return (
    <form className={styles.form} onSubmit={submit} noValidate>
      {FIELDS.map((field) => {
        const setup = FIELD_SETUP[field];
        const problem = shownProblem(field);
        const hintId = field === 'phone' ? `${id}-${field}-hint` : undefined;
        const errorId = problem ? `${id}-${field}-error` : undefined;
        return (
          <div key={field} className={styles.field}>
            <label className={styles.label} htmlFor={`${id}-${field}`}>
              {t(setup.label)}
              {field === 'email' && <span className={styles.optional}>({t('common.optional')})</span>}
            </label>
            <input
              ref={inputs[field]}
              id={`${id}-${field}`}
              className={styles.input}
              type={setup.type}
              inputMode={setup.inputMode}
              autoComplete={setup.autoComplete}
              value={values[field]}
              disabled={disabled}
              required={field !== 'email'}
              aria-invalid={problem !== undefined}
              aria-describedby={[errorId, hintId].filter(Boolean).join(' ') || undefined}
              onChange={(e) => onChange({ ...values, [field]: e.target.value })}
              onBlur={() => setTouched((prev) => (prev[field] ? prev : { ...prev, [field]: true }))}
            />
            {problem && (
              <p id={errorId} className={styles.fieldError}>
                {t(`form.error.${field}.${problem}` as MessageKey)}
              </p>
            )}
            {hintId && (
              <p id={hintId} className={styles.hint}>
                {t('form.phoneHint')}
              </p>
            )}
          </div>
        );
      })}

      {formError && <Notice tone="danger">{formError}</Notice>}

      <Button type="submit" block busy={busy} disabled={disabled}>
        {busy ? t('form.submitting') : t('form.submit')}
      </Button>
      <p className={styles.consent}>{t('form.consent')}</p>
    </form>
  );
}
