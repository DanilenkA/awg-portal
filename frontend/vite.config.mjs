import { fileURLToPath, URL } from 'url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    //
    outDir: process.env.DIST_OUT_DIR || '../internal/app/api/core/frontend-dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (
            id.includes('/node_modules/vue/') ||
            id.includes('/node_modules/@vue/') ||
            id.includes('/node_modules/pinia/') ||
            id.includes('/node_modules/vue-router/') ||
            id.includes('/node_modules/vue-i18n/')
          ) {
            return 'vendor-vue'
          }
          if (id.includes('/node_modules/bootstrap/') || id.includes('/node_modules/@popperjs/')) {
            return 'vendor-bootstrap'
          }
          if (id.includes('/node_modules/@fortawesome/')) {
            return 'vendor-fontawesome'
          }
        },
      },
    },
  },
  css: {
    preprocessorOptions: {
      scss: {
        quietDeps: true,
        silenceDeprecations: ['color-functions', 'global-builtin', 'import', 'if-function'],
      },
    },
  },
  // local dev api (proxy to avoid cors problems)
  server: {
    port: 5000,
    proxy: {
      "/api/v0": {
        target: "http://10.130.130.207:8123",
        changeOrigin: true,
        secure: false,
        withCredentials: true,
        headers: {
          "x-wg-dev": true,
        },
        rewrite: (path) => path,
      },
      "/app": {
        target: "http://10.130.130.207:8123",
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
