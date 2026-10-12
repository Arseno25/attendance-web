import type { Result, Session } from '~/types/auth'

export function useAuth() {
  const session = useState<Session | null>('auth-session', () => null)
  const request = useApi()
  async function load() {
    try {
      session.value = (await request<Result<Session>>('/auth/me')).data
    }
    catch (error) {
      if ((error as { status?: number }).status !== 401) throw error
      session.value = null
    }
    return session.value
  }
  async function login(email: string, password: string) {
    session.value = (await request<Result<Session>>('/auth/login', { method: 'POST', body: { email, password } })).data
  }
  async function logout() {
    await request('/auth/logout', { method: 'POST', body: {} })
    session.value = null
    await navigateTo('/login')
  }
  return { session, load, login, logout }
}
