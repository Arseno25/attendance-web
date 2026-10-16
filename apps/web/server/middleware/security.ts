import { securityHeaders } from '../utils/securityHeaders'

export default defineEventHandler((event) => {
  const nonce = btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(24))))
  event.context.cspNonce = nonce
  setResponseHeaders(event, securityHeaders(nonce, useRuntimeConfig(event).public.apiBase))
})
