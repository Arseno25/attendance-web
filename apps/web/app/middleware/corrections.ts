export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().has('corrections.request', 'attendance.approve', 'attendance.manual'))
    return navigateTo('/admin?forbidden=1')
})
