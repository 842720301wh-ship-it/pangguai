import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  build: { outDir: fileURLToPath(new URL('../dist/web', import.meta.url)), emptyOutDir: true },
  server: {
    port: 5173,
    strictPort: true,
    proxy: { '/api': 'http://127.0.0.1:18080' },
  },
})
