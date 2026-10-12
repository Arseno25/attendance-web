import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  modules: ['shadcn-nuxt', '@nuxt/eslint'],
  compatibilityDate: '2026-10-12',
  devtools: { enabled: false },
  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss()] },
  shadcn: { prefix: '', componentDir: '@/components/ui' },
  routeRules: { '/login': { ssr: false }, '/admin': { ssr: false }, '/admin/**': { ssr: false } },
  runtimeConfig: {
    public: { apiBase: 'http://localhost:8080/api/v1' },
  },
  app: {
    head: {
      htmlAttrs: { lang: 'id' },
      title: 'Absensi Sekolah',
      meta: [{ name: 'description', content: 'Sistem absensi sekolah dengan pencatatan dan laporan terpusat.' }],
    },
  },
})
