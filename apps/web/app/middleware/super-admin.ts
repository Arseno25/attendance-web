export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  const session = useAuth().session.value
  if (session?.user.role !== 'super_admin' || !session.permissions.includes('roles.manage'))
    return navigateTo('/admin?forbidden=1')
})
