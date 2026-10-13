import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// The API the dev and preview servers proxy /v1 to, so that the widget can
// use relative URLs exactly as it does when the Go server hosts it.
const apiTarget = process.env.SLOTBOOK_API_TARGET ?? 'http://localhost:8080';

function basePath(raw: string | undefined): string {
  if (!raw) return '/';
  const withLeading = raw.startsWith('/') ? raw : `/${raw}`;
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`;
}

export default defineConfig({
  base: basePath(process.env.BASE_PATH),
  plugins: [react()],
  server: {
    // The mock imports ../seed/demo.json, outside this package.
    fs: { allow: ['..'] },
    proxy: { '/v1': apiTarget },
  },
  preview: {
    proxy: { '/v1': apiTarget },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
});
