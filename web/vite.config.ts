import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { mockApi } from './mock/servidor.ts'

const conMock = process.env.MOCK === '1' || process.env.VITE_MOCK === '1'

export default defineConfig({
  plugins: [react(), ...(conMock ? [mockApi()] : [])],
  server: {
    port: 5173,
    proxy: conMock
      ? undefined
      : {
          '/api': {
            target: process.env.API_URL ?? 'http://127.0.0.1:8080',
            changeOrigin: true,
          },
        },
  },
  preview: {
    port: 4173,
  },
  build: {
    outDir: 'dist',
  },
})
