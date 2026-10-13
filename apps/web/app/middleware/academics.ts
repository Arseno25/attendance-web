export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().session.value?.permissions.includes('academics.read')) return navigateTo('/admin?forbidden=1')
})
