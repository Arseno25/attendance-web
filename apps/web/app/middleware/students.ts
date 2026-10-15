export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  if (!useAuth().has('students.read')) return navigateTo('/admin?forbidden=1')
})
