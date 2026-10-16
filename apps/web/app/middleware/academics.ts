export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().has('academics.read')) return navigateTo('/admin?forbidden=1')
})
