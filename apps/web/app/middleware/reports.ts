export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  const user = useAuth().session.value?.user
  if (!user || !['super_admin', 'admin', 'teacher'].includes(user.role)) return navigateTo('/admin?forbidden=1')
})
