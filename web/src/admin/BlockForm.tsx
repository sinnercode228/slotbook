import { useId, useState, type FormEvent } from 'react';
import type { AdminApi } from '../api/client';
import { errorCode } from '../api/errors';
import type { Staff } from '../api/types';
import { Button, Notice } from '../components/ui';
import { useErrorMessage } from '../components/useErrorMessage';
import { useI18n } from '../i18n/useI18n';
import type { LocalDate } from '../schedule/date';
import { parseTimeOfDay } from '../schedule/timeOfDay';
import { formatInstant, instantAt } from '../schedule/zone';
import styles from './Admin.module.css';

interface BlockFormProps {
  admin: AdminApi;
  date: LocalDate;
  zone: string;
  staff: Staff[];
  onCreated: () => void;
}

type Result = { tone: 'success' | 'danger'; text: string } | null;

/** Takes a specialist out of the schedule for part of the shown day. */
export function BlockForm({ admin, date, zone, staff, onCreated }: BlockFormProps) {
  const { t } = useI18n();
  const errorMessage = useErrorMessage();
  const id = useId();
  const [staffId, setStaffId] = useState<number | null>(null);
  const [from, setFrom] = useState('13:00');
  const [to, setTo] = useState('14:00');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result>(null);
  const member = staff.find((s) => s.id === staffId) ?? staff[0];

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!member) return;
    const start = instantAt(date, parseTimeOfDay(from), zone);
    const end = to === '00:00' ? instantAt(date, 24 * 60, zone) : instantAt(date, parseTimeOfDay(to), zone);
    if (end <= start) {
      setResult({ tone: 'danger', text: t('admin.block.order') });
      return;
    }
    setBusy(true);
    setResult(null);
    try {
      await admin.createBlock({ staffId: member.id, start: formatInstant(start, zone), end: formatInstant(end, zone), note: note.trim() });
      setResult({ tone: 'success', text: t('admin.block.done', { from, to, name: member.name }) });
      setNote('');
      onCreated();
    } catch (err) {
      setResult({ tone: 'danger', text: errorCode(err) === 'slot_taken' ? t('admin.block.overlap') : errorMessage(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className={styles.panel} aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className={styles.panelTitle}>
        {t('admin.block.title')}
      </h2>
      <form className={styles.blockForm} onSubmit={submit}>
        <div className={styles.field}>
          <label className={styles.label} htmlFor={`${id}-staff`}>
            {t('admin.block.staff')}
          </label>
          <select
            id={`${id}-staff`}
            className={styles.select}
            value={member?.id ?? ''}
            onChange={(e) => setStaffId(Number(e.target.value))}
          >
            {staff.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </div>
        <div className={styles.timeRow}>
          <div className={styles.field}>
            <label className={styles.label} htmlFor={`${id}-from`}>
              {t('admin.block.from')}
            </label>
            <input
              id={`${id}-from`}
              className={styles.input}
              type="time"
              step={900}
              required
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </div>
          <div className={styles.field}>
            <label className={styles.label} htmlFor={`${id}-to`}>
              {t('admin.block.to')}
            </label>
            <input
              id={`${id}-to`}
              className={styles.input}
              type="time"
              step={900}
              required
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </div>
        </div>
        <div className={styles.field}>
          <label className={styles.label} htmlFor={`${id}-note`}>
            {t('admin.block.note')}
          </label>
          <input
            id={`${id}-note`}
            className={styles.input}
            value={note}
            maxLength={200}
            placeholder={t('admin.block.notePlaceholder')}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        {result && <Notice tone={result.tone}>{result.text}</Notice>}
        <Button type="submit" variant="secondary" busy={busy} disabled={!member}>
          {t('admin.block.submit')}
        </Button>
      </form>
    </section>
  );
}
