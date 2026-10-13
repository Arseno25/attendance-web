export default defineNuxtRouteMiddleware(async () => {
  if (import.meta.server) return
  const auth = useAuth()
  try {
    if (!(await auth.load())) return navigateTo('/login')
  } catch {
    return navigateTo('/login?unavailable=1')
  }
})
