import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { loginSchema } from '../apps/web/app/lib/validation/auth.ts'
import { useFormValidation } from '../apps/web/app/composables/useFormValidation.ts'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { computed, ref, watch, effectScope, nextTick } = requireWeb('vue')
const directory = resolve('.tools/navigation-login-regression')
mkdirSync(directory, { recursive: true })
writeFileSync(directory + '/package.json', JSON.stringify({ type: 'module' }))
try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') }
catch (error) { if (error.code !== 'EEXIST') throw error }

async function component(name, file) {
  const { descriptor } = parse(readFileSync(file, 'utf8'), { filename: file })
  const script = compileScript(descriptor, { id: name })
  let source = script.content.replace(/^import (NavMain|NavUser) from .*$/gm, 'const $1 = {}')
  source = source.replaceAll('~/lib/validation/auth', pathToFileURL(resolve('apps/web/app/lib/validation/auth.ts')).href)
  const target = `${directory}/${name}.ts`
  writeFileSync(target, source)
  return (await import(pathToFileURL(target).href)).default
}
const Sidebar = await component('Sidebar', 'apps/web/app/components/AppSidebar.vue')
const Login = await component('Login', 'apps/web/app/components/login-04/components/LoginForm.vue')
Object.assign(globalThis, { computed, ref, watch, useFormValidation })

test('sidebar groups only authorized links, hides empty categories and reacts to access changes', () => {
  const session = ref({ user: { role: 'super_admin' }, permissions: ['academics.read', 'attendance.read', 'devices.health', 'biometrics.enroll'] })
  globalThis.useAuth = () => ({ session })
  const page = Sidebar.setup({}, { expose() {} })
  assert.deepEqual(page.groups.value.map(g => g.title), ['Utama', 'Akademik', 'Kehadiran', 'Administrasi'])
  const links = page.groups.value.flatMap(g => g.items.map(i => i.url))
  assert.equal(new Set(links).size, links.length)
  assert.equal(links.length, 11)
  session.value = { user: { role: 'operator' }, permissions: ['biometrics.enroll'] }
  assert.deepEqual(page.groups.value.map(g => g.title), ['Utama', 'Kehadiran'])
  assert.deepEqual(page.groups.value.flatMap(g => g.items.map(i => i.url)), ['/admin', '/admin/biometrics'])
  session.value = { user: { role: 'teacher' }, permissions: ['academics.read', 'attendance.read'] }
  assert.ok(page.groups.value.flatMap(g => g.items).some(i => i.url === '/admin/reports'))
  assert.ok(!page.groups.value.some(g => g.title === 'Administrasi'))
})

function loginContext(login, query = {}) {
  let navigation = ''
  Object.assign(globalThis, {
    useAuth: () => ({ login }), useRoute: () => ({ query }),
    apiMessage: error => error.message,
    navigateTo: async path => { navigation = path },
  })
  const scope = effectScope()
  const page = scope.run(() => Login.setup({}, { expose() {} }))
  return { page, scope, navigation: () => navigation }
}

test('login validates required fields inline without requesting the server or applying new-password composition rules', async () => {
  let calls = 0
  const { page, scope } = loginContext(async () => { calls++ })
  try {
    await page.submit()
    assert.equal(calls, 0)
    assert.equal(page.errors.value.email, 'Email wajib diisi.')
    assert.equal(page.errors.value.password, 'Password wajib diisi.')
    assert.equal(page.failure.value, '')
    assert.equal(loginSchema.safeParse({ email: 'staff@example.test', password: 'legacy' }).success, true)
    page.email.value = 'invalid'; page.password.value = 'legacy'
    await nextTick(); await page.submit()
    assert.equal(calls, 0)
    assert.equal(page.errors.value.email, 'Masukkan email yang valid.')
  } finally { scope.stop() }
})

test('failed login stays in the form, preserves input for retry and clears stale server errors on editing', async () => {
  const { page, scope, navigation } = loginContext(async () => { throw new Error('Email atau password tidak sesuai.') })
  try {
    page.email.value = 'staff@example.test'; page.password.value = 'existing'
    await nextTick(); await page.submit()
    assert.equal(page.failure.value, 'Email atau password tidak sesuai.')
    assert.equal(page.email.value, 'staff@example.test')
    assert.equal(page.password.value, 'existing')
    assert.equal(page.busy.value, false)
    assert.equal(navigation(), '')
    page.password.value = 'corrected'
    await nextTick(); assert.equal(page.failure.value, '')
  } finally { scope.stop() }
})

test('successful login blocks duplicate submission, clears password and navigates without a toast', async () => {
  let complete, calls = 0
  const { page, scope, navigation } = loginContext(() => { calls++; return new Promise(resolve => { complete = resolve }) }, { expired: '1' })
  try {
    assert.match(page.message.value, /Sesi berakhir/)
    page.email.value = 'staff@example.test'; page.password.value = 'existing'
    await nextTick()
    const pending = page.submit(); await page.submit()
    assert.equal(calls, 1); assert.equal(page.busy.value, true)
    complete(); await pending
    assert.equal(page.password.value, '')
    assert.equal(page.busy.value, false)
    assert.equal(navigation(), '/admin')
    assert.equal(page.failure.value, '')
  } finally { scope.stop() }
})
process.on('exit', () => rmSync(directory, { recursive: true, force: true }))
