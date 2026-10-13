export function useAttendance() {
  const { request, options, dateTime } = useMasterData()
  const auth = useAuth()
  const has = (permission: string) => auth.session.value?.permissions.includes(permission) ?? false
  const canManual = computed(() => has('attendance.manual'))
  const canApprove = computed(() => has('attendance.approve'))
  const canConfigure = computed(() => has('academics.write'))
  const canRequest = computed(() => has('corrections.request') || canManual.value)
  const today = () => new Intl.DateTimeFormat('en-CA', { timeZone: auth.session.value?.school.timezone ?? 'Asia/Jakarta', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
  return { request, options, dateTime, canManual, canApprove, canConfigure, canRequest, today }
}
