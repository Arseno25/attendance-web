export default defineNuxtRouteMiddleware(() => {
  if (!useAuth().session.value?.permissions.includes('devices.health')) return navigateTo('/admin')
})
