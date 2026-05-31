import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      // ShopHub backend (port-forward the shophub service here in dev).
      '/api': { target: 'http://localhost:8090', changeOrigin: true },
    },
  },
});
