import { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { useApi } from '../app/ApiContext';
import { ApiError, errorCode } from '../api/errors';
import type { Confirmation, Customer, Hold, Location, Service, Staff } from '../api/types';
import { Button, Notice } from '../components/ui';
import { useErrorMessage } from '../components/useErrorMessage';
import { problemFromServerMessage, type CustomerField, type CustomerProblem } from '../domain/customer';
import { useI18n } from '../i18n/useI18n';
import { formatTime, visitorZone as detectVisitorZone, zonesDiffer } from '../lib/format';
import { randomId } from '../lib/randomId';
import { LocationChoice, ServiceChoice, StaffChoice } from './ChoiceSteps';
import { ConfirmationView } from './ConfirmationView';
import { emptyContact, type ContactValues } from './contact';
import { ContactForm } from './ContactForm';
import { HoldTimer } from './HoldTimer';
import { loadProgress, saveProgress } from './progress';
import { SlotPicker } from './SlotPicker';
import { StepHeader } from './StepHeader';
import { SummaryBar, SummaryCard, type EditableStep, type SummaryProps } from './Summary';
import styles from './Widget.module.css';

type Step = EditableStep | 'details' | 'done';

interface ActiveHold {
  hold: Hold;
  /** Local clock time at which the hold runs out; immune to a skewed device clock. */
  deadline: number;
  totalMs: number;
  /** One key per hold, so a retried confirm is answered with the first result. */
  idempotencyKey: string;
}

type TimeNotice = { kind: 'taken'; start: string } | { kind: 'invalid' } | { kind: 'expired' } | { kind: 'failed'; error: unknown };

interface State {
  step: Step;
  location: Location | null;
  service: Service | null;
  /** undefined: not chosen yet; null: any available specialist. */
  staff: Staff | null | undefined;
  staffList: Staff[];
  hold: ActiveHold | null;
  holdExpired: boolean;
  notice: TimeNotice | null;
  hidden: string[];
  confirmation: Confirmation | null;
}

type Action =
  | { type: 'location'; location: Location }
  | { type: 'service'; service: Service }
  | { type: 'staff'; staff: Staff | null; staffList: Staff[] }
  | { type: 'goto'; step: Step }
  | { type: 'held'; hold: ActiveHold }
  | { type: 'holdFailed'; start: string; error: unknown }
  | { type: 'holdExpired' }
  | { type: 'pickAgain'; notice: TimeNotice | null }
  | { type: 'confirmed'; confirmation: Confirmation }
  | { type: 'restart' };

const initial: State = {
  step: 'location',
  location: null,
  service: null,
  staff: undefined,
  staffList: [],
  hold: null,
  holdExpired: false,
  notice: null,
  hidden: [],
  confirmation: null,
};

const STEP_NUMBER: Record<Exclude<Step, 'done'>, number> = { location: 1, service: 2, staff: 3, time: 4, details: 5 };
const PREVIOUS: Partial<Record<Step, Step>> = { service: 'location', staff: 'service', time: 'staff', details: 'time' };

/** Whether a saved state has everything its step needs. */
function isComplete(s: State): boolean {
  switch (s.step) {
    case 'location':
      return true;
    case 'service':
      return Boolean(s.location);
    case 'staff':
      return Boolean(s.location && s.service);
    case 'time':
      return Boolean(s.location && s.service) && s.staff !== undefined;
    case 'details':
      return Boolean(s.location && s.service && s.hold);
    case 'done':
      return Boolean(s.location && s.service && s.confirmation);
    default:
      return false;
  }
}

/** The state saved before a reload, without the notice: it may hold an error object JSON cannot keep. */
function restoredState(saved: State | undefined): State {
  return saved && isComplete(saved) ? { ...initial, ...saved, notice: null } : initial;
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case 'location':
      return action.location.id === state.location?.id
        ? { ...state, step: 'service' }
        : { ...initial, step: 'service', location: action.location };
    case 'service':
      return action.service.id === state.service?.id
        ? { ...state, step: 'staff' }
        : { ...state, step: 'staff', service: action.service, staff: undefined, hold: null, notice: null, hidden: [] };
    case 'staff': {
      const same = state.staff !== undefined && (action.staff?.id ?? null) === (state.staff?.id ?? null);
      return same
        ? { ...state, step: 'time', staffList: action.staffList }
        : { ...state, step: 'time', staff: action.staff, staffList: action.staffList, hold: null, notice: null, hidden: [] };
    }
    case 'goto': {
      // Leaving the contact form gives the hold up; see releaseHold.
      const dropped = state.step === 'details' ? { hold: null, holdExpired: false } : {};
      return { ...state, ...dropped, step: action.step, notice: action.step === 'time' ? state.notice : null };
    }
    case 'held':
      return { ...state, step: 'details', hold: action.hold, holdExpired: false, notice: null };
    case 'holdFailed': {
      const code = errorCode(action.error);
      if (code === 'slot_taken') {
        return { ...state, notice: { kind: 'taken', start: action.start }, hidden: [...state.hidden, action.start] };
      }
      if (code === 'invalid_slot') return { ...state, notice: { kind: 'invalid' }, hidden: [...state.hidden, action.start] };
      return { ...state, notice: { kind: 'failed', error: action.error } };
    }
    case 'holdExpired':
      return { ...state, holdExpired: true };
    case 'pickAgain':
      return { ...state, step: 'time', hold: null, holdExpired: false, notice: action.notice };
    case 'confirmed':
      return { ...state, step: 'done', confirmation: action.confirmation, hold: null };
    case 'restart':
      return initial;
  }
}

export function BookingWidget() {
  const api = useApi();
  const { t, locale } = useI18n();
  const errorMessage = useErrorMessage();
  const [saved] = useState(() => loadProgress<State>());
  const [state, dispatch] = useReducer(reducer, saved?.state, restoredState);
  const [pendingStart, setPendingStart] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [contact, setContact] = useState<ContactValues>(() => saved?.contact ?? emptyContact);
  const [confirming, setConfirming] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [serverProblem, setServerProblem] = useState<{ field: CustomerField; problem: CustomerProblem } | null>(null);
  const [ownZone] = useState(detectVisitorZone);
  const containerRef = useRef<HTMLDivElement>(null);
  const firstRender = useRef(true);

  const { step, location, service, staff, hold } = state;

  useEffect(() => saveProgress({ state, contact }), [state, contact]);

  // Move focus and the viewport to the new step, as a page change would.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    const node = containerRef.current;
    if (!node) return;
    const heading = node.querySelector<HTMLElement>('h2');
    heading?.setAttribute('tabindex', '-1');
    heading?.focus({ preventScroll: true });
    if (node.getBoundingClientRect().top < 0) node.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }, [step]);

  const startShown = state.confirmation?.booking.start ?? hold?.hold.booking.start ?? null;
  const differentZone =
    location && startShown && zonesDiffer(location.time_zone, ownZone, Date.parse(startShown)) ? ownZone : null;

  const staffName = (id: number) => state.staffList.find((s) => s.id === id)?.name ?? '';

  async function pickSlot(start: string) {
    if (!service) return;
    if (hold && !state.holdExpired && hold.hold.booking.start === start) {
      dispatch({ type: 'goto', step: 'details' });
      return;
    }
    setPendingStart(start);
    try {
      const created = await api.createHold({ serviceId: service.id, staffId: staff?.id ?? null, start });
      const totalMs = Date.parse(created.expires_at) - Date.parse(created.booking.created_at);
      dispatch({
        type: 'held',
        hold: { hold: created, deadline: Date.now() + totalMs, totalMs, idempotencyKey: randomId() },
      });
      setFormError(null);
      setServerProblem(null);
    } catch (error) {
      dispatch({ type: 'holdFailed', start, error });
      setRefreshKey((n) => n + 1);
    } finally {
      setPendingStart(null);
    }
  }

  async function confirm(customer: Customer) {
    if (!hold) return;
    setConfirming(true);
    setFormError(null);
    setServerProblem(null);
    try {
      const confirmation = await api.confirmHold(hold.hold.hold_token, customer, hold.idempotencyKey);
      dispatch({ type: 'confirmed', confirmation });
    } catch (error) {
      const code = errorCode(error);
      if (code === 'hold_expired' || code === 'hold_not_found') {
        dispatch({ type: 'pickAgain', notice: { kind: 'expired' } });
        setRefreshKey((n) => n + 1);
        return;
      }
      const problem = code === 'validation_failed' && error instanceof ApiError ? problemFromServerMessage(error.message) : null;
      if (problem) setServerProblem(problem);
      else setFormError(code === 'network' ? t('form.error.network') : errorMessage(error));
    } finally {
      setConfirming(false);
    }
  }

  const onHoldExpired = useCallback(() => dispatch({ type: 'holdExpired' }), []);

  /**
   * Gives the running hold back, so that its time is free at once, for others
   * and for a visitor who now wants a time overlapping it. Nobody waits for
   * the answer: if the call fails, the hold runs out on its own.
   */
  function releaseHold() {
    if (!hold || state.holdExpired) return;
    void api
      .releaseHold(hold.hold.booking.id, hold.hold.hold_token)
      .catch((error: unknown) => console.error('Could not release the hold', error))
      // The list may have been loaded while the hold still counted as busy.
      .finally(() => setRefreshKey((n) => n + 1));
  }

  function goBack(target: Step) {
    if (step === 'details') releaseHold();
    dispatch({ type: 'goto', step: target });
  }

  function timeNotice() {
    const notice = state.notice;
    if (!notice || !location) return null;
    switch (notice.kind) {
      case 'taken':
        return <Notice tone="warning">{t('slots.taken', { time: formatTime(notice.start, location.time_zone, locale) })}</Notice>;
      case 'invalid':
        return <Notice tone="warning">{t('slots.invalid')}</Notice>;
      case 'expired':
        return <Notice tone="warning">{t('slots.expired')}</Notice>;
      case 'failed':
        return <Notice tone="danger">{errorMessage(notice.error)}</Notice>;
    }
  }

  const back = PREVIOUS[step] ? () => goBack(PREVIOUS[step]!) : undefined;
  const header = (title: string) =>
    step !== 'done' && <StepHeader step={STEP_NUMBER[step]} title={title} onBack={back} />;

  function body() {
    switch (step) {
      case 'location':
        return (
          <>
            {header(t('step.location'))}
            <LocationChoice current={location} onChoose={(l) => dispatch({ type: 'location', location: l })} />
          </>
        );
      case 'service':
        return (
          location && (
            <>
              {header(t('step.service'))}
              <ServiceChoice location={location} current={service} onChoose={(s) => dispatch({ type: 'service', service: s })} />
            </>
          )
        );
      case 'staff':
        return (
          location &&
          service && (
            <>
              {header(t('step.staff'))}
              <StaffChoice
                location={location}
                service={service}
                current={staff}
                onChoose={(s, list) => dispatch({ type: 'staff', staff: s, staffList: list })}
              />
            </>
          )
        );
      case 'time':
        return (
          location &&
          service && (
            <>
              {header(t('step.time'))}
              <SlotPicker
                location={location}
                serviceId={service.id}
                staffId={staff?.id ?? null}
                onPick={pickSlot}
                pendingStart={pendingStart}
                heldStart={hold && !state.holdExpired ? hold.hold.booking.start : null}
                hiddenStarts={state.hidden}
                notice={timeNotice()}
                refreshKey={refreshKey}
              />
            </>
          )
        );
      case 'details':
        return (
          location &&
          hold && (
            <>
              {header(t('step.details'))}
              <div className={styles.stack}>
                {!state.holdExpired && (
                  <HoldTimer
                    start={hold.hold.booking.start}
                    zone={location.time_zone}
                    visitorZone={differentZone}
                    deadline={hold.deadline}
                    totalMs={hold.totalMs}
                    onExpire={onHoldExpired}
                  />
                )}
                {state.holdExpired ? (
                  <Notice
                    tone="warning"
                    title={t('hold.expiredTitle')}
                    action={
                      <Button
                        variant="secondary"
                        size="small"
                        onClick={() => {
                          dispatch({ type: 'pickAgain', notice: null });
                          setRefreshKey((n) => n + 1);
                        }}
                      >
                        {t('hold.pickAgain')}
                      </Button>
                    }
                  >
                    {t('hold.expiredBody')}
                  </Notice>
                ) : (
                  <ContactForm
                    values={contact}
                    onChange={(values) => {
                      setContact(values);
                      setServerProblem(null);
                    }}
                    onSubmit={confirm}
                    busy={confirming}
                    formError={formError}
                    serverProblem={serverProblem}
                  />
                )}
              </div>
            </>
          )
        );
      case 'done':
        return (
          state.confirmation &&
          location &&
          service && (
            <ConfirmationView
              confirmation={state.confirmation}
              location={location}
              service={service}
              staffName={staffName(state.confirmation.booking.staff_id)}
              visitorZone={differentZone}
              onBookAnother={() => {
                dispatch({ type: 'restart' });
                setContact(emptyContact);
              }}
            />
          )
        );
    }
  }

  if (step === 'done') {
    return (
      <div className={`${styles.card} ${styles.narrow}`} ref={containerRef}>
        {body()}
      </div>
    );
  }

  const activeHold = hold && !state.holdExpired ? hold : null;
  const summary: SummaryProps = {
    location,
    service,
    // Once a hold exists, it names the person who will do the job.
    staffName: activeHold ? staffName(activeHold.hold.booking.staff_id) : staff === undefined ? undefined : (staff?.name ?? null),
    start: activeHold?.hold.booking.start ?? null,
    visitorZone: differentZone,
  };

  return (
    <div className={styles.layout} ref={containerRef}>
      <div className={styles.main}>
        <SummaryBar {...summary} onEdit={goBack} />
        <div className={styles.card}>{body()}</div>
      </div>
      <SummaryCard {...summary} />
    </div>
  );
}
