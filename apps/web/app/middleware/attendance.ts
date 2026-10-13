export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  const permissions = useAuth().session.value?.permissions ?? []
  if (!permissions.includes('attendance.read') && !permissions.includes('attendance.manual')) return navigateTo('/admin?forbidden=1')
})
