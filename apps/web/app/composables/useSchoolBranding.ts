import type { SchoolBranding } from '~/types/settings'
import type { Result } from '~/types/auth'

export function useSchoolBranding() {
  const branding = useState<SchoolBranding>('school-branding', () => ({
    name: '',
    app_name: 'Absensi Sekolah',
    logo_url: '',
  }))
  const request = useApi()
  async function load() {
    try {
      branding.value = (await request<Result<SchoolBranding>>('/school/branding')).data
    } catch {
      /* The sign-in form remains available while school details cannot load. */
    }
  }
  return { branding, load }
}
