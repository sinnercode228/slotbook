/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** "mock" runs the API in the browser; anything else talks to the server. */
  readonly VITE_API_MODE?: string;
  /** Base URL of the API; empty means the page's own origin. */
  readonly VITE_API_URL?: string;
  /** The business name shown in the header. */
  readonly VITE_BRAND_NAME?: string;
}
