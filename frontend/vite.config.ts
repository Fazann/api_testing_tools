import vue from '@vitejs/plugin-vue';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [vue()],
  resolve: {
    // `@/…` imports resolve from src/ and `@wails/…` from the generated Wails bindings
    // (root-relative, so no Node path helpers are needed).
    alias: { '@/': '/src/', '@wails/': '/wailsjs/' },
  },
  build: {
    // The desktop webview loads assets locally; keep output simple.
    target: 'es2020',
    sourcemap: false,
  },
  test: {
    environment: 'happy-dom',
  },
});
