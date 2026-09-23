import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// В dev-режиме /api проксируется на запущенный стек:
//   API_URL=https://localhost npm run dev   (docker compose up, сертификат Caddy не проверяется)
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: process.env.API_URL ?? 'https://localhost', changeOrigin: true, secure: false },
    },
  },
});
