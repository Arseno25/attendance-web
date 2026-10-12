import type { Session } from '~/types/auth'

export function useApi() {
  const config = useRuntimeConfig()
  const session = useState<Session | null>('auth-session', () => null)
  return async <T>(path: string, options: { method?: 'GET' | 'POST' | 'PATCH', body?: object, query?: Record<string, string | number> } = {}) => {
    try {
      return await $fetch<T>(`${config.public.apiBase}${path}`, {
        ...options,
        credentials: 'include',
        retry: 0,
        timeout: 8000,
        headers: options.method && options.method !== 'GET'
          ? { 'X-CSRF-Token': session.value?.csrf_token ?? '' }
          : {},
      })
    }
    catch (error) {
      const status = (error as { status?: number }).status
      if (status === 401) {
        session.value = null
        if (import.meta.client && window.location.pathname.startsWith('/admin')) {
          await navigateTo('/login?expired=1')
        }
      }
      throw error
    }
  }
}

export function apiMessage(error: unknown) {
  return (error as { data?: { error?: { message?: string } } })?.data?.error?.message
    ?? 'Layanan belum tersedia. Periksa koneksi lalu coba kembali.'
}
