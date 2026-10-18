import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // 开发时把 /api 请求转发到 Go 后端
    proxy: { '/api': 'http://localhost:8080' },
  },
})
