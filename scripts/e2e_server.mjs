import assert from 'node:assert/strict'
import { spawn, execFile } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { randomUUID } from 'node:crypto'
import { promisify } from 'node:util'

const run = promisify(execFile)
const root = resolve(import.meta.dirname, '..')
const schema = `browser_test_${randomUUID().replaceAll('-', '')}`
const dsn = process.env.E2E_DATABASE_URL
assert(dsn, 'Set E2E_DATABASE_URL explicitly to a local disposable-test PostgreSQL database')
const url = new URL(dsn)
assert(['localhost', '127.0.0.1', '[::1]'].includes(url.hostname), 'Browser fixture database must be local')
url.searchParams.set('options', `-csearch_path=${schema}`)
const pgEnv = (scoped) => ({
  ...process.env,
  PGHOST: url.hostname.replaceAll(/[\[\]]/g, ''), PGPORT: url.port || '5432',
  PGUSER: decodeURIComponent(url.username), PGPASSWORD: decodeURIComponent(url.password),
  PGDATABASE: decodeURIComponent(url.pathname.slice(1)), PGSSLMODE: url.searchParams.get('sslmode') || 'prefer',
  PGOPTIONS: scoped ? `-csearch_path=${schema}` : '',
})
const env = {
  ...process.env, DATABASE_URL: url.href, WEB_ORIGIN: 'http://localhost:3007',
  API_ADDRESS: '127.0.0.1:8087', AI_BASE_URL: '', ALLOW_LOCAL_WEB_PORT_RANGE: '0', TRUSTED_PROXY_CIDRS: '',
}
const sql = (text, scoped = true) => run('psql', ['-X', '-q', '-v', 'ON_ERROR_STOP=1', '-c', text], {
  env: pgEnv(scoped), cwd: root,
})
async function bootstrap() {
  const child = spawn(resolve(root, '.tools/bootstrap'), ['--email', 'browser-admin@example.test', '--name', 'Browser admin', '--school-name', 'Browser School'], { cwd: root, env, stdio: ['pipe', 'ignore', 'pipe'] })
  child.stderr.resume()
  child.stdin.end('BrowserTest1!')
  return (await once(child, 'exit'))[0]
}
let api, created = false
const migrationProbe = await mkdtemp(resolve(tmpdir(), 'attendance-migration-probe-'))
try {
  await sql(`CREATE SCHEMA "${schema}"`, false)
  created = true
  await run(resolve(root, '.tools/goose'), ['-dir', resolve(root, 'services/api/migrations'), 'up'], { cwd: root, env: { ...env, GOOSE_DRIVER: 'postgres', GOOSE_DBSTRING: url.href } })
  try {
    await run(resolve(root, '.tools/api'), [], { cwd: root, env, timeout: 5000 })
    assert.fail('API started before school bootstrap')
  } catch (error) {
    assert.equal(error.code, 1)
    assert.match(error.stdout + error.stderr, /no school configured/)
  }
  const migrations = await readdir(resolve(root, 'services/api/migrations'))
  for (const file of migrations) {
    if (file.endsWith('.sql')) await symlink(resolve(root, 'services/api/migrations', file), resolve(migrationProbe, file))
  }
  await writeFile(resolve(migrationProbe, `${String(Math.max(...migrations.filter(file => file.endsWith('.sql')).map(file => Number(file.split('_')[0]))) + 1).padStart(5, '0')}_expected_failure.sql`), '-- +goose Up\nSELECT nonexistent_migration_probe();\n-- +goose Down\nSELECT 1;\n')
  await assert.rejects(run(resolve(root, '.tools/goose'), ['-dir', migrationProbe, 'up'], { env: { ...env, GOOSE_DRIVER: 'postgres', GOOSE_DBSTRING: url.href } }), error => /nonexistent_migration_probe/.test(error.stderr + error.stdout))
  assert.equal(await bootstrap(), 0, 'first bootstrap must succeed')
  assert.equal(await bootstrap(), 1, 'repeat bootstrap must fail')
  await sql(await readFile(resolve(root, 'services/api/tests/e2e.sql'), 'utf8'))
  api = spawn(resolve(root, '.tools/api'), [], { cwd: root, env, stdio: ['ignore', 'inherit', 'inherit'] })
  for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => api?.kill('SIGTERM'))
  const [code] = await once(api, 'exit')
  process.exitCode = code ?? 0
} finally {
  api?.kill('SIGTERM')
  if (created) await sql(`DROP SCHEMA "${schema}" CASCADE`, false)
  await rm(migrationProbe, { recursive: true, force: true })
}
