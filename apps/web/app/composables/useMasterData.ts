import type { Result } from '~/types/auth'

export function useMasterData(writePermission = 'academics.write') {
  const request = useApi()
  const auth = useAuth()
  const canWrite = computed(() => auth.has(writePermission))
  async function options<T>(path: string, query: Record<string, string | number> = {}) {
    const items: T[] = []
    for (let page = 1; ; page++) {
      const result = await request<Result<T[]>>(path, { query: { ...query, page, page_size: 100 } })
      items.push(...result.data)
      if (items.length >= (result.pagination?.total ?? items.length)) return items
    }
  }
  const dateTime = (value: string) =>
    new Intl.DateTimeFormat('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: auth.session.value?.school.timezone ?? 'Asia/Jakarta',
    }).format(new Date(value))
  return { request, canWrite, options, dateTime }
}
