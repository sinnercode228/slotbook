import { AdminPage } from '../admin/AdminPage';
import { buttonClass } from '../components/buttonClass';
import { LanguageSwitch } from '../components/LanguageSwitch';
import { DemoBar } from '../demo/DemoBar';
import { useI18n } from '../i18n/useI18n';
import { initials } from '../lib/initials';
import { ManagePage } from '../manage/ManagePage';
import { BookingWidget } from '../widget/BookingWidget';
import styles from './App.module.css';
import { BRAND_NAME } from './brand';
import { useRoute, type Route } from './router';

function Screen({ route }: { route: Route }) {
  const { t } = useI18n();
  if (route.path === '/') return <BookingWidget />;
  if (route.path === '/admin') return <AdminPage />;
  const manage = /^\/booking\/([^/]+)$/.exec(route.path);
  if (manage) {
    const id = decodeURIComponent(manage[1]!);
    const token = route.query.get('token') ?? '';
    return <ManagePage key={`${id}:${token}`} bookingId={id} manageToken={token} notice={route.query.get('notice')} />;
  }
  return (
    <section className={styles.notFound}>
      <h1>{t('app.notFound.title')}</h1>
      <p>{t('app.notFound.body')}</p>
      <a className={buttonClass('secondary')} href="#/">
        {t('app.notFound.home')}
      </a>
    </section>
  );
}

export function App() {
  const route = useRoute();
  const { t } = useI18n();
  const wide = route.path === '/admin';

  return (
    <div className={`${styles.page} ${wide ? styles.wide : ''}`}>
      <DemoBar />
      <header className={styles.header}>
        <div className={styles.headerInner}>
          <a className={styles.brand} href="#/">
            <span className={styles.mark} aria-hidden="true">
              {initials(BRAND_NAME)}
            </span>
            <span className={styles.brandText}>
              <span className={styles.brandName}>{BRAND_NAME}</span>
              <span className={styles.brandSub}>{wide ? t('admin.title') : t('app.bookVisit')}</span>
            </span>
          </a>
          <LanguageSwitch />
        </div>
      </header>
      <main className={styles.main}>
        <Screen route={route} />
      </main>
      <footer className={styles.footer}>
        <div className={styles.footerInner}>
          <span>{t('app.poweredBy')}</span>
          <a href={wide ? '#/' : '#/admin'}>{wide ? t('app.bookVisit') : t('app.staffLogin')}</a>
        </div>
      </footer>
    </div>
  );
}
