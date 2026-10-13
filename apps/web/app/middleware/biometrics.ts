export default defineNuxtRouteMiddleware(() => {
  if (import.meta.server) return
  const permissions = useAuth().session.value?.permissions ?? []
  if (!permissions.includes('biometrics.enroll') && !permissions.includes('biometrics.revoke'))
    return navigateTo('/admin?forbidden=1')
})
