export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().has('attendance.read', 'reports.export')) return navigateTo('/admin?forbidden=1')
})
