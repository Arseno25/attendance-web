import assert from 'node:assert/strict'
import { test } from 'node:test'

test('dependency release gate rejects new advisories, runtime packages and expired exceptions', async () => {
  const { reviewAudit } = await import('./check_web_audit.mjs')
  const advisory = 'https://github.com/advisories/KNOWN'
  const audit = { metadata: { vulnerabilities: { total: 1 } }, vulnerabilities: { tool: { via: [{ url: advisory, name: 'tool' }] } } }
  const policy = { expires: '2026-11-15', advisories: [{ url: advisory, package: 'tool', reason: 'Build tool only' }] }
  assert.equal(reviewAudit(audit, policy, new Set(), new Date('2026-10-15')).length, 1)
  assert.throws(() => reviewAudit(audit, policy, new Set(['tool']), new Date('2026-10-15')), /runtime/)
  assert.throws(() => reviewAudit(audit, policy, new Set(), new Date('2026-12-01')), /expired/)
  const changed = structuredClone(audit)
  changed.vulnerabilities.tool.via[0].url = 'https://github.com/advisories/NEW'
  assert.throws(() => reviewAudit(changed, policy, new Set(), new Date('2026-10-15')), /Unreviewed/)
  assert.throws(() => reviewAudit({ error: 'registry unavailable' }, policy, new Set()), /audit/)
})
