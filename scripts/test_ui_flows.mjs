import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript, compileTemplate } = requireWeb('@vue/compiler-sfc')
const { ref, nextTick, watch } = requireWeb('vue')
const flush = async () => { await nextTick(); await new Promise(r => setImmediate(r)); await nextTick() }

test('all authored Vue templates compile after formatting', () => {
  for (const file of readdirSync('apps/web/app', { recursive: true })) {
    if (!file.endsWith('.vue') || file.startsWith('components/ui/')) continue
    const filename = resolve('apps/web/app', file)
    const { descriptor, errors } = parse(readFileSync(filename, 'utf8'), { filename })
    assert.deepEqual(errors, [], file)
    if (!descriptor.template) continue
    const compiled = compileTemplate({ source: descriptor.template.content, filename, id: file, isProd: true, compilerOptions: { expressionPlugins: ['typescript'] } })
    assert.deepEqual(compiled.errors, [], file)
  }
})

test('status badges distinguish attendance, review and revoked states', async () => {
  const { statusVariant } = await import('../apps/web/app/lib/status.ts')
  assert.notEqual(statusVariant('present'), statusVariant('late'))
  assert.notEqual(statusVariant('approved'), statusVariant('rejected'))
  assert.equal(statusVariant('not_recorded'), 'secondary')
  assert.equal(statusVariant('unknown'), 'secondary')
})

test('dialog focus returns to a row trigger and respects removed targets or cancelled events', async () => {
  const { dialogFocus } = await import('../apps/web/app/lib/dialogFocus.ts')
  const previousDocument = globalThis.document
  let focused = 0
  const trigger = { isConnected: true, focus: () => focused++ }
  globalThis.document = {
    activeElement: { closest: () => ({ getAttribute: () => 'row-trigger' }) },
    getElementById: id => id === 'row-trigger' ? trigger : null,
  }
  try {
    const focus = dialogFocus(); focus.onOpenAutoFocus()
    const event = new Event('close', { cancelable: true }); focus.onCloseAutoFocus(event)
    assert.equal(focused, 1); assert.equal(event.defaultPrevented, true)
    const cancelled = new Event('close', { cancelable: true }); cancelled.preventDefault(); focus.onCloseAutoFocus(cancelled)
    trigger.isConnected = false; focus.onCloseAutoFocus(new Event('close', { cancelable: true }))
    assert.equal(focused, 1)
    trigger.isConnected = true; globalThis.document.activeElement = { ...trigger, closest: () => null }
    focus.onOpenAutoFocus(); focus.onCloseAutoFocus(new Event('close', { cancelable: true }))
    assert.equal(focused, 2)
  }
  finally { globalThis.document = previousDocument }
})

let mounted, unmounted
function context(request) {
  mounted = undefined; unmounted = undefined
  Object.assign(globalThis, { ref, watch, nextTick, onMounted: f => { mounted = f }, onBeforeUnmount: f => { unmounted = f }, useApi: () => request, useMasterData: () => ({ options: async path => (await request(path)).data }), useRuntimeConfig: () => ({ public: { apiBase: 'http://localhost:8080/api/v1' } }), definePageMeta: () => {}, useHead: () => {}, useAuth: () => ({ session: ref({ permissions: ['devices.health'] }), load: async () => true }) })
  globalThis.document = Object.assign(new EventTarget(), { hidden: false, querySelector: () => null, querySelectorAll: () => [] })
  globalThis.window = new EventTarget()
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { onLine: true } })
}
async function kioskSetup() {
  const directory = resolve('.tools/ui-regression')
  mkdirSync(directory, { recursive: true })
  try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') } catch (e) { if (e.code !== 'EEXIST') throw e }
  const source = readFileSync('apps/web/app/pages/absensi.vue', 'utf8')
  const { descriptor } = parse(source)
  const compiled = compileScript(descriptor, { id: 'kiosk-regression' })
  const file = directory + '/kiosk.ts'
  writeFileSync(file, compiled.content.replaceAll('../lib/validation/biometrics-devices.ts', pathToFileURL(resolve('apps/web/app/lib/validation/biometrics-devices.ts')).href).replaceAll('../composables/useFormValidation.ts', pathToFileURL(resolve('apps/web/app/composables/useFormValidation.ts')).href).replaceAll('../lib/biometrics.ts', pathToFileURL(resolve('apps/web/app/lib/biometrics.ts')).href))
  const module = await import(pathToFileURL(file).href + '?' + Math.random())
  return module.default.setup({}, { expose() {} })
}
const session = { device_name: 'Gate', session_name: 'Morning', expires_at: new Date(Date.now() + 60000).toISOString(), csrf_token: 'csrf', recognition_enabled: false }

test('default operator discovers an eligible session without attendance grant', async () => {
  context(async path => {
    if (path === '/kiosk/me') throw new Error('unpaired')
    if (path === '/kiosk/session-options') return { data: [{ id: 'open', name: 'Morning', class_name: 'Class A' }] }
    throw new Error('403 attendance grant absent')
  })
  const page = await kioskSetup()
  try { await mounted(); await flush(); assert.equal(page.sessions.value.length, 1); assert.equal(page.sessions.value[0].id, 'open') }
  finally { unmounted() }
})

test('resumed kiosk reloads pairing choices when its session ends', async () => {
  context(async path => {
    if (path === '/kiosk/me') return { data: session }
    if (path === '/kiosk/session-options') return { data: [{ id: 'next', name: 'Next', class_name: 'Class A' }] }
    throw new Error('unexpected endpoint')
  })
  const page = await kioskSetup()
  try { await mounted(); page.stop('expired'); await flush(); assert.equal(page.sessions.value.length, 1); assert.equal(page.sessions.value[0].id, 'next') }
  finally { unmounted() }
})

test('late mount result does not create timers after leaving kiosk', async () => {
  let finish
  context(() => new Promise(r => { finish = r }))
  const page = await kioskSetup()
  const originalInterval = globalThis.setInterval; const originalTimeout = globalThis.setTimeout
  let timers = 0
  globalThis.setInterval = (...args) => { timers++; return originalInterval(...args) }
  globalThis.setTimeout = (...args) => { timers++; return originalTimeout(...args) }
  try {
    const pending = mounted(); unmounted(); finish({ data: session }); await pending
    assert.equal(timers, 0)
  }
  finally { page.stop(); globalThis.setInterval = originalInterval; globalThis.setTimeout = originalTimeout }
})

test('live invalidation survives a blocked editing form and refetches after dismissal', async () => {
  context(async () => {})
  let socket
  globalThis.WebSocket = class { constructor() { socket = this } close() {} }
  const { useAttendanceLive } = await import(pathToFileURL(resolve('apps/web/app/composables/useAttendanceLive.ts')).href)
  let calls = 0
  const blocked = ref(true)
  useAttendanceLive('session', async () => { calls++ }, blocked)
  try {
    mounted(); socket.onmessage({ data: JSON.stringify({ event: 'attendance.changed' }) }); await flush()
    assert.equal(calls, 0)
    blocked.value = false; await flush(); assert.equal(calls, 1)
  }
  finally { unmounted() }
})
process.on('exit', () => rmSync(resolve('.tools/ui-regression'), { recursive: true, force: true }))

test('successful pairing clears operator identity while retaining device session', async () => {
  context(async path => {
    if (path === '/kiosk/me') throw new Error('unpaired')
    if (path === '/kiosk/session-options') return { data: [{ id: 'open', name: 'Morning', class_name: 'Class A' }] }
    if (path === '/kiosk/session') return { data: session }
    throw new Error('unexpected endpoint')
  })
  const page = await kioskSetup()
  try { await mounted(); page.pairingToken.value = 'a'.repeat(64); page.sessionId.value = '11111111-1111-4111-8111-111111111111'; await page.pair(); assert.equal(page.auth.session.value, null); assert.equal(page.kiosk.value.device_name, 'Gate') }
  finally { unmounted() }
})

test('invalid pairing does not call the API and corrected fields clear errors', async () => {
  let calls = 0
  context(async () => { calls++; return { data: session } })
  const page = await kioskSetup()
  try {
    await page.pair(); assert.equal(calls, 0); assert.ok(page.errors.value.pairing_token); assert.ok(page.errors.value.session_id)
    page.pairingToken.value = 'a'.repeat(64); page.sessionId.value = '11111111-1111-4111-8111-111111111111'; await flush()
    assert.deepEqual(page.errors.value, {}); await page.pair(); assert.equal(calls, 1)
  }
  finally { unmounted() }
})

test('a failed old heartbeat cannot discard a newly paired device session', async () => {
  context(async () => ({ data: session }))
  const originalInterval = globalThis.setInterval; const originalFetch = globalThis.$fetch
  const callbacks = []; let reject
  globalThis.setInterval = fn => { callbacks.push(fn); return 0 }
  globalThis.$fetch = () => new Promise((_, failure) => { reject = failure })
  const page = await kioskSetup()
  try {
    await mounted(); const oldHeartbeat = callbacks[0]()
    page.arm({ ...session, csrf_token: 'new-csrf' })
    reject(new Error('old session expired')); await oldHeartbeat
    assert.equal(page.kiosk.value.csrf_token, 'new-csrf')
    assert.equal(page.message.value, '')
  }
  finally { unmounted(); globalThis.setInterval = originalInterval; globalThis.$fetch = originalFetch }
})

test('late recognition cannot render names after camera stops and uses device CSRF', async () => {
  context(async () => ({ data: session }))
  let finish, options
  const originalFetch = globalThis.$fetch
  globalThis.$fetch = (_url, input) => { options = input; return new Promise(resolve => { finish = resolve }) }
  const page = await kioskSetup()
  try {
    await mounted(); page.camera.value = { capture: () => '/9j/AA==', stop() {} }
    page.cameraChanged(true)
    assert.equal(options.headers['X-CSRF-Token'], 'csrf')
    assert.equal(options.credentials, 'include')
    assert.match(options.body.event_id, /^[0-9a-f-]{36}$/)
    page.stopCamera()
    finish({ data: { decision: 'accepted', recognition_enabled: true, attendance: { outcome: 'recorded' }, faces: [{ box: { x: 0, y: 0, width: 0.5, height: 0.5 }, label: 'Old identity', student_id: 'old' }] } })
    await flush(); assert.deepEqual(page.faces.value, []); assert.equal(page.cameraActive.value, false)
  }
  finally { unmounted(); globalThis.$fetch = originalFetch }
})
