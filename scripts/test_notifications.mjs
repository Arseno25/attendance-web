import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { ref, reactive, computed } = requireWeb('vue')
const { toast } = await import(pathToFileURL(resolve('apps/web/node_modules/vue-sonner/lib/index.js')).href)
const directory = resolve('.tools/notification-regression')
mkdirSync(directory, { recursive: true })
writeFileSync(directory + '/package.json', JSON.stringify({ type: 'module' }))
try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') }
catch (error) { if (error.code !== 'EEXIST') throw error }
const { descriptor } = parse(readFileSync('apps/web/app/pages/admin/account.vue', 'utf8'))
const script = compileScript(descriptor, { id: 'notifications' })
const filename = directory + '/account.ts'
writeFileSync(filename, script.content.replaceAll('~/lib/validation/auth', pathToFileURL(resolve('apps/web/app/lib/validation/auth.ts')).href).replaceAll('~/types/auth', pathToFileURL(resolve('apps/web/app/types/auth.ts')).href))
const module = await import(pathToFileURL(filename).href)
function page(saveProfile, valid = true) {
  Object.assign(globalThis, {
    ref, reactive, computed, definePageMeta() {}, useHead() {},
    useAuth: () => ({ session: ref({ user: { email: 'staff@example.test', full_name: 'Petugas' } }), saveProfile }),
    useFormValidation: () => ({ errors: ref({}), validate: () => valid, clear() {} }),
    apiMessage: error => error.data?.error?.message ?? 'Layanan belum tersedia.',
  })
  return module.default.setup({}, { expose() {} })
}
function since(offset) { return toast.getHistory().slice(offset) }

test('Sonner success is emitted only after the operation completes, with duplicate submissions blocked', async () => {
  let complete, calls = 0
  const component = page(() => { calls++; return new Promise(resolve => { complete = resolve }) })
  const offset = toast.getHistory().length
  const saving = component.save()
  assert.equal(component.busy.value, true)
  assert.deepEqual(since(offset), [])
  await component.save(); assert.equal(calls, 1)
  complete(); await saving
  assert.equal(component.busy.value, false)
  assert.equal(since(offset).length, 1)
  assert.equal(since(offset)[0].type, 'success')
  assert.equal(since(offset)[0].title, 'Akun berhasil diperbarui.')
})

test('failed operations emit one error and no success, preserving the form for retry', async () => {
  const component = page(async () => { throw { data: { error: { message: 'Layanan sedang sibuk.' } } } })
  component.form.full_name = 'Nama untuk dicoba kembali'
  const offset = toast.getHistory().length
  await component.save()
  assert.equal(component.form.full_name, 'Nama untuk dicoba kembali')
  assert.equal(component.busy.value, false)
  assert.equal(since(offset).length, 1)
  assert.equal(since(offset)[0].type, 'error')
  assert.equal(since(offset)[0].title, 'Layanan sedang sibuk.')
})

test('field validation failures do not send requests or create redundant toasts', async () => {
  let calls = 0
  const component = page(async () => { calls++ }, false)
  const offset = toast.getHistory().length
  await component.save()
  assert.equal(calls, 0)
  assert.deepEqual(since(offset), [])
})
process.on('exit', () => rmSync(directory, { recursive: true, force: true }))
