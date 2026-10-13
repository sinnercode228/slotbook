import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

function basePath(raw: string | undefined): string {
  if (!raw) return '/';
  const withLeading = raw.startsWith('/') ? raw : `/${raw}`;
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`;
}

export default defineConfig(({ mode }) => {
  // Shell variables and .env files alike; see .env.example.
  const env = loadEnv(mode, process.cwd(), '');
  // The dev and preview servers proxy /v1 here, so that the widget uses
  // relative URLs exactly as it does when the Go server hosts it.
  const apiTarget = env.SLOTBOOK_API_TARGET || 'http://localhost:8080';

  return {
    base: basePath(env.BASE_PATH),
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
  };
});
