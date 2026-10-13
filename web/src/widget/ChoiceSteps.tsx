import { useApi } from '../app/ApiContext';
import type { Location, Service, Staff } from '../api/types';
import { ChevronRight, Clock, MapPin, Users } from '../components/icons';
import { ErrorState, SkeletonList } from '../components/ui';
import { useI18n } from '../i18n/useI18n';
import { formatMoney } from '../lib/format';
import { initials } from '../lib/initials';
import { useResource, type Resource } from '../lib/useResource';
import styles from './Widget.module.css';

function Loaded<T>({ resource, children }: { resource: Resource<T>; children: (data: T) => React.ReactNode }) {
  const { t } = useI18n();
  if (resource.data !== undefined) return <>{children(resource.data)}</>;
  if (resource.error !== undefined) return <ErrorState error={resource.error} onRetry={resource.reload} />;
  return <SkeletonList label={t('common.loading')} />;
}

function Empty({ text }: { text: string }) {
  return <p className={styles.empty}>{text}</p>;
}

export function LocationChoice({ current, onChoose }: { current: Location | null; onChoose: (l: Location) => void }) {
  const api = useApi();
  const { t } = useI18n();
  const locations = useResource('locations', () => api.listLocations());
  return (
    <Loaded resource={locations}>
      {(list) =>
        list.length === 0 ? (
          <Empty text={t('location.empty')} />
        ) : (
          <ul className={styles.options}>
            {list.map((loc) => (
              <li key={loc.id}>
                <button type="button" className={styles.option} aria-current={loc.id === current?.id} onClick={() => onChoose(loc)}>
                  <span className={`${styles.avatar} ${styles.avatarAccent}`} aria-hidden="true">
                    <MapPin />
                  </span>
                  <span className={styles.optionMain}>
                    <span className={styles.optionTitle}>{loc.name}</span>
                    <span className={styles.optionMeta}>{loc.address}</span>
                  </span>
                  <ChevronRight className={styles.chevron} />
                </button>
              </li>
            ))}
          </ul>
        )
      }
    </Loaded>
  );
}

export function ServiceChoice({
  location,
  current,
  onChoose,
}: {
  location: Location;
  current: Service | null;
  onChoose: (s: Service) => void;
}) {
  const api = useApi();
  const { t, locale } = useI18n();
  const services = useResource(`services|${location.id}`, () => api.listServices(location.id));
  return (
    <Loaded resource={services}>
      {(list) =>
        list.length === 0 ? (
          <Empty text={t('service.empty')} />
        ) : (
          <ul className={styles.options}>
            {list.map((svc) => (
              <li key={svc.id}>
                <button type="button" className={styles.option} aria-current={svc.id === current?.id} onClick={() => onChoose(svc)}>
                  <span className={styles.optionMain}>
                    <span className={styles.optionTitle}>{svc.name}</span>
                    <span className={styles.optionMeta}>
                      <span>
                        <Clock />
                        {t('common.minutes', { n: svc.duration_minutes })}
                      </span>
                    </span>
                  </span>
                  <span className={styles.optionAside}>{formatMoney(svc.price, locale)}</span>
                  <ChevronRight className={styles.chevron} />
                </button>
              </li>
            ))}
          </ul>
        )
      }
    </Loaded>
  );
}

export function StaffChoice({
  location,
  service,
  current,
  onChoose,
}: {
  location: Location;
  service: Service;
  /** undefined: nothing chosen yet; null: any available. */
  current: Staff | null | undefined;
  onChoose: (staff: Staff | null, everyone: Staff[]) => void;
}) {
  const api = useApi();
  const { t } = useI18n();
  const staff = useResource(`staff|${location.id}|${service.id}`, () => api.listStaff(location.id, service.id));
  return (
    <Loaded resource={staff}>
      {(list) =>
        list.length === 0 ? (
          <Empty text={t('staff.empty')} />
        ) : (
          <ul className={styles.options}>
            <li>
              <button type="button" className={styles.option} aria-current={current === null} onClick={() => onChoose(null, list)}>
                <span className={`${styles.avatar} ${styles.avatarAccent}`} aria-hidden="true">
                  <Users />
                </span>
                <span className={styles.optionMain}>
                  <span className={styles.optionTitle}>{t('staff.any')}</span>
                  <span className={styles.optionMeta}>{t('staff.anyHint')}</span>
                </span>
                <ChevronRight className={styles.chevron} />
              </button>
            </li>
            {list.map((member) => (
              <li key={member.id}>
                <button
                  type="button"
                  className={styles.option}
                  aria-current={member.id === current?.id}
                  onClick={() => onChoose(member, list)}
                >
                  <span className={styles.avatar} aria-hidden="true">
                    {initials(member.name)}
                  </span>
                  <span className={styles.optionMain}>
                    <span className={styles.optionTitle}>{member.name}</span>
                  </span>
                  <ChevronRight className={styles.chevron} />
                </button>
              </li>
            ))}
          </ul>
        )
      }
    </Loaded>
  );
}
