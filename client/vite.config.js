import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import seo from './vite.seo.js'

// The backend allows the origin in its FRONTEND_URL, so the dev server must stay on port 3000.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')
  return {
    plugins: [react(), seo(env.VITE_SITE_URL || 'https://layr.appmd.dev')],
    server: { port: 3000, strictPort: true },
    preview: { port: 3000, strictPort: true },
    build: { target: 'es2020', sourcemap: false },
  }
})
