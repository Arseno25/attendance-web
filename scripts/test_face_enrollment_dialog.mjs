import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { useFormValidation } from '../apps/web/app/composables/useFormValidation.ts'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { ref, computed, watch, reactive, nextTick, effectScope } = requireWeb('vue')
const flush = async () => { await nextTick(); await new Promise(r => setImmediate(r)); await nextTick() }
const ready = { enrolled: false, enrollment_allowed: true, pipeline_ready: true, recognition_enabled: false }
const directory = resolve('.tools/face-dialog-tests')

async function setup(request, permissions = ['biometrics.enroll', 'biometrics.revoke']) {
  let unmount
  Object.assign(globalThis, { ref, computed, watch, useFormValidation, onBeforeUnmount: fn => { unmount = fn }, useApi: () => request, useAuth: () => ({ session: ref({ permissions }) }), apiMessage: () => 'Request failed' })
  mkdirSync(directory, { recursive: true })
  try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') } catch (e) { if (e.code !== 'EEXIST') throw e }
  const { descriptor } = parse(readFileSync('apps/web/app/components/StudentFaceEnrollment.vue', 'utf8'))
  const compiled = compileScript(descriptor, { id: 'face-dialog' })
  const filename = directory + '/dialog.ts'
  writeFileSync(filename, compiled.content.replaceAll('~/lib/validation/biometrics-devices', pathToFileURL(resolve('apps/web/app/lib/validation/biometrics-devices.ts')).href))
  const module = await import(pathToFileURL(filename).href + '?' + Math.random())
  const props = reactive({ studentId: 'student-a', open: true })
  const updates = []
  const scope = effectScope()
  const page = scope.run(() => module.default.setup(props, { expose() {}, emit: (_, value) => updates.push(value) }))
  return { page, props, updates, stop: () => { unmount(); scope.stop() } }
}

test('single enrollment modal uses server eligibility and rejects insufficient frames', async () => {
  let writes = 0
  const { page, stop } = await setup(async (_path, options) => { if (options.method) writes++; return { data: ready } })
  try {
    await flush()
    assert.equal(page.enrollmentReady.value, true)
    await page.save()
    assert.equal(writes, 0)
    assert.ok(page.errors.value.frames)
  } finally { stop() }
})

test('closing and switching students abort status requests and discard old results', async () => {
  const requests = []
  const { page, props, stop } = await setup((_path, options) => new Promise(finish => requests.push({ signal: options.signal, finish })))
  try {
    props.studentId = 'student-b'
    await flush()
    assert.equal(requests[0].signal.aborted, true)
    requests[0].finish({ data: ready })
    await flush()
    assert.equal(page.status.value, null)
    props.open = false
    await flush()
    assert.equal(requests[1].signal.aborted, true)
    requests[1].finish({ data: ready })
    await flush()
    assert.equal(page.status.value, null)
    assert.deepEqual(page.frames.value, [])
  } finally { stop() }
})

test('revoker cannot enroll and pending writes cannot close the modal', async () => {
  let writes = 0
  const { page, updates, stop } = await setup(async () => { writes++; return { data: ready } }, ['biometrics.revoke'])
  try {
    await flush()
    assert.equal(page.enrollmentReady.value, false)
    await page.save()
    assert.equal(writes, 1)
    page.busy.value = true
    page.close(false)
    assert.deepEqual(updates, [])
    page.busy.value = false
    page.close(false)
    assert.deepEqual(updates, [false])
  } finally { stop() }
})
process.on('exit', () => rmSync(directory, { recursive: true, force: true }))
