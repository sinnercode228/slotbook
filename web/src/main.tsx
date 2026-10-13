import '@fontsource-variable/inter';
import './styles/global.css';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { ApiContext } from './app/ApiContext';
import { App } from './app/App';
import { createApi } from './app/createApi';
import { I18nProvider } from './i18n/I18nProvider';

void createApi().then((api) => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <ApiContext.Provider value={api}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </ApiContext.Provider>
    </StrictMode>,
  );
});
