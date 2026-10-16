export function securityHeaders(nonce: string, apiBase: string) {
  const api = new URL(apiBase)
  if (!['http:', 'https:'].includes(api.protocol)) throw new Error('Invalid public API URL')
  const websocket = new URL(api.origin)
  websocket.protocol = api.protocol === 'https:' ? 'wss:' : 'ws:'
  return {
    'Content-Security-Policy': [
      "default-src 'self'",
      `script-src 'self' 'nonce-${nonce}'`,
      "style-src 'self' 'unsafe-inline'",
      `connect-src 'self' ${api.origin} ${websocket.origin}`,
      `img-src 'self' data: blob: ${api.origin}`,
      "font-src 'self' data:",
      "media-src 'self' blob:",
      "worker-src 'self' blob:",
      "object-src 'none'",
      "base-uri 'self'",
      "frame-ancestors 'none'",
      "form-action 'self'",
    ].join('; '),
    'X-Frame-Options': 'DENY',
    'X-Content-Type-Options': 'nosniff',
    'Referrer-Policy': 'same-origin',
    'Permissions-Policy': 'camera=(self), microphone=(), geolocation=()',
    'Strict-Transport-Security': 'max-age=31536000',
  }
}
