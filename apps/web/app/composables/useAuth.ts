import type { Result, Session } from '~/types/auth'

const readPermission: Record<string, string> = {
  'students.read': 'students.write',
  'academics.read': 'academics.write',
  'devices.health': 'devices.manage',
}

export function useAuth() {
  const session = useState<Session | null>('auth-session', () => null)
  const request = useApi()
  const has = (...required: string[]) =>
    required.some((key) => {
      const permissions = session.value?.permissions ?? []
      return permissions.includes(key) || permissions.includes(readPermission[key] ?? '')
    })
  async function load() {
    try {
      session.value = (await request<Result<Session>>('/auth/me')).data
    } catch (error) {
      if ((error as { status?: number }).status !== 401) throw error
      session.value = null
    }
    return session.value
  }
  async function login(email: string, password: string) {
    session.value = (
      await request<Result<Session>>('/auth/login', { method: 'POST', body: { email, password } })
    ).data
  }
  async function saveProfile(body: {
    full_name: string
    email: string
    current_password?: string
    new_password?: string
  }) {
    session.value = (
      await request<Result<Session>>('/auth/profile', { method: 'PATCH', body })
    ).data
  }
  async function logout() {
    await request('/auth/logout', { method: 'POST', body: {} })
    session.value = null
    await navigateTo('/login')
  }
  return { session, has, load, login, logout, saveProfile }
}
