import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { computed, ref } = requireWeb('vue')
const { useAuth } = await import('../apps/web/app/composables/useAuth.ts')

async function guard(name, permissions) {
  globalThis.defineNuxtRouteMiddleware = fn => fn
  const session = ref({ user: { role: 'custom_school_role' }, permissions })
  globalThis.useState = () => session
  globalThis.useApi = () => async () => ({ data: [] })
  globalThis.useAuth = useAuth
  globalThis.navigateTo = path => path
  const source = readFileSync(`apps/web/app/middleware/${name}.ts`, 'utf8')
  return (await import(`data:text/javascript,${encodeURIComponent(source)}`)).default()
}

test('custom roles open reports by permission, not built-in role name', async () => {
  assert.equal(await guard('reports', ['attendance.read']), undefined)
  assert.equal(await guard('reports', ['reports.export']), undefined)
  assert.equal(await guard('reports', ['students.read']), '/admin?forbidden=1')
})

test('students and corrections have their own permission guards', async () => {
  assert.equal(await guard('students', ['students.read']), undefined)
  assert.equal(await guard('students', ['academics.read']), '/admin?forbidden=1')
  assert.equal(await guard('corrections', ['attendance.approve']), undefined)
  assert.equal(await guard('corrections', ['corrections.request']), undefined)
  assert.equal(await guard('corrections', ['attendance.read']), '/admin?forbidden=1')
})

test('student write actions follow student permission independently of academic writes', async () => {
  globalThis.computed = computed
  globalThis.useApi = () => async () => ({ data: [] })
  const auth = { session: ref({ permissions: ['students.write'] }) }
  globalThis.useState = () => auth.session
  globalThis.useAuth = useAuth
  const { useMasterData } = await import('../apps/web/app/composables/useMasterData.ts')
  assert.equal(useMasterData('students.write').canWrite.value, true)
  assert.equal(useMasterData().canWrite.value, false)
  auth.session.value.permissions = ['academics.write']
  assert.equal(useMasterData('students.write').canWrite.value, false)
  assert.equal(useMasterData().canWrite.value, true)
})
