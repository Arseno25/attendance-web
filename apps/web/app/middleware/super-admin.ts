export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (useAuth().session.value?.user.role !== 'super_admin') return navigateTo('/admin?forbidden=1')
})
