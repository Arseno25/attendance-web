export function useAttendanceLive(sessionId: string, refetch: () => Promise<void>, blocked?: Ref<boolean>) {
  const state = ref('Menghubungkan live…')
  const config = useRuntimeConfig()
  let socket: WebSocket | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  let refreshing = false
  let pending = false
  async function refresh() {
    pending = true
    if (refreshing) return
    refreshing = true
    try { while (pending && !disposed && !blocked?.value) { pending = false; await refetch() } }
    finally { refreshing = false }
  }
  function close() { clearTimeout(retry); const old = socket; socket = undefined; old?.close() }
  function connect() {
    close()
    if (disposed || document.hidden || !navigator.onLine) { state.value = 'Live dijeda'; return }
    const url = new URL(`${config.public.apiBase}/ws/attendance`)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'; url.searchParams.set('session_id', sessionId)
    socket = new WebSocket(url)
    const active = socket
    active.onopen = () => { state.value = 'Live terhubung'; void refresh() }
    active.onmessage = (event) => { if (active !== socket) return; try { if (JSON.parse(event.data).event === 'attendance.changed') void refresh() } catch { state.value = 'Notifikasi tidak valid' } }
    active.onclose = (event) => {
      if (active !== socket) return
      state.value = event.code === 1008 ? 'Sesi atau akses tidak aktif' : 'Live terputus · data dapat dimuat ulang'
      if (event.code !== 1008 && !disposed && !document.hidden && navigator.onLine) retry = setTimeout(connect, 5000)
    }
  }
  function visibility() { if (document.hidden) { close(); state.value = 'Live dijeda' } else connect() }
  if (blocked) watch(blocked, value => { if (!value && pending) void refresh() })
  onMounted(() => { connect(); document.addEventListener('visibilitychange', visibility); window.addEventListener('online', connect); window.addEventListener('offline', visibility) })
  onBeforeUnmount(() => { disposed = true; close(); document.removeEventListener('visibilitychange', visibility); window.removeEventListener('online', connect); window.removeEventListener('offline', visibility) })
  return state
}
