import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

// The interface only ever requests origin-relative paths, because an
// Authorization header is not CORS-safelisted and Bench Core answers no
// preflight. The development server therefore forwards /api to the configured
// Bench Core endpoint; an appliance serves the built assets from Bench Core
// itself, where the same paths are already same-origin.
export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, '.', 'VITE_NILO_');
  const target = environment.VITE_NILO_API_BASE_URL || 'http://127.0.0.1:8090';
  const proxy = { '/api': { target, changeOrigin: false } };

  return {
    plugins: [react()],
    server: { proxy },
    preview: { proxy },
    test: {
      environment: 'jsdom',
      setupFiles: './src/test/setup.ts',
      // Only *.test.ts belongs to this runner. The integrated browser suite
      // is *.spec.ts and is collected by playwright.config.ts instead; the
      // default pattern would otherwise try to run it here, in jsdom.
      include: ['src/**/*.test.{ts,tsx}', 'e2e/**/*.test.ts'],
    },
  };
});
