import vue from '@vitejs/plugin-vue';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [vue()],
  build: {
    // The desktop webview loads assets locally; keep output simple.
    target: 'es2020',
    sourcemap: false,
  },
  test: {
    environment: 'happy-dom',
  },
});
