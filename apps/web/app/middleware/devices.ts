export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().has('devices.health')) return navigateTo('/admin?forbidden=1')
})
