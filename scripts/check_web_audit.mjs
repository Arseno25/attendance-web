import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export function reviewAudit(audit, policy, runtimePackages, now = new Date()) {
  if (!audit.metadata?.vulnerabilities || !audit.vulnerabilities) throw new Error('Invalid npm audit result; registry failures are not clean scans')
  const reviewed = []
  for (const [name, vulnerability] of Object.entries(audit.vulnerabilities)) {
    if (runtimePackages.has(name)) throw new Error(`Affected package found in runtime output: ${name}`)
    for (const source of vulnerability.via) {
      if (typeof source === 'string') continue
      const exception = policy.advisories.find(item => item.url === source.url && item.package === source.name)
      if (!exception?.reason) throw new Error(`Unreviewed advisory: ${source.url}`)
      const expiry = Date.parse(policy.expires)
      if (!Number.isFinite(expiry) || now >= expiry) throw new Error(`Dependency exception expired: ${source.url}`)
      if (runtimePackages.has(source.name)) throw new Error(`Affected package found in runtime output: ${source.name}`)
      reviewed.push(source.url)
    }
  }
  return [...new Set(reviewed)]
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [auditFile, serverDirectory] = process.argv.slice(2)
  if (!auditFile || !serverDirectory) throw new Error('Usage: node scripts/check_web_audit.mjs AUDIT_JSON BUILT_SERVER_DIRECTORY')
  const read = path => JSON.parse(readFileSync(path, 'utf8'))
  const output = read(resolve(serverDirectory, 'package.json'))
  const packages = new Set(Object.keys(output.dependencies ?? {}))
  for (const file of readdirSync(serverDirectory, { recursive: true })) {
    if (file.endsWith('/package.json')) {
      const pkg = read(resolve(serverDirectory, file))
      if (pkg.name) packages.add(pkg.name)
    }
  }
  const policy = read(new URL('../deploy/dependency-exceptions.json', import.meta.url))
  const reviewed = reviewAudit(read(auditFile), policy, packages)
  console.log(`${reviewed.length} known build-tool advisories reviewed; none of their packages found in built runtime metadata. Exceptions expire ${policy.expires}.`)
}
