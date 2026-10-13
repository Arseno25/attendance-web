import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { ref, nextTick, watch } = requireWeb('vue')
const flush = async () => { await nextTick(); await new Promise(r => setImmediate(r)); await nextTick() }
let mounted, unmounted
function context(request) {
  mounted = undefined; unmounted = undefined
  Object.assign(globalThis, { ref, watch, nextTick, onMounted: f => { mounted = f }, onBeforeUnmount: f => { unmounted = f }, useApi: () => request, useMasterData: () => ({ options: async path => (await request(path)).data }), useRuntimeConfig: () => ({ public: { apiBase: 'http://localhost:8080/api/v1' } }), definePageMeta: () => {}, useHead: () => {}, useAuth: () => ({ session: ref({ permissions: ['devices.health'] }), load: async () => true }) })
  globalThis.document = Object.assign(new EventTarget(), { hidden: false })
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
  writeFileSync(file, compiled.content)
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
  try { await mounted(); page.pairingToken.value = 'a'.repeat(64); page.sessionId.value = 'open'; await page.pair(); assert.equal(page.auth.session.value, null); assert.equal(page.kiosk.value.device_name, 'Gate') }
  finally { unmounted() }
})
