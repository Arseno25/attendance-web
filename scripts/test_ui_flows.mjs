import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript, compileTemplate } = requireWeb('@vue/compiler-sfc')
const { ref, nextTick, watch, computed } = requireWeb('vue')
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
  Object.assign(globalThis, { requestAnimationFrame:fn=>{fn();return 0}, ref, computed, watch, nextTick, onMounted: f => { mounted = f }, onBeforeUnmount: f => { unmounted = f }, useApi: () => request, useMasterData: () => ({ options: async path => (await request(path)).data }), useRuntimeConfig: () => ({ public: { apiBase: 'http://localhost:8080/api/v1' } }), definePageMeta: () => {}, useHead: () => {}, useAuth: () => ({ session: ref({ permissions: ['devices.health'] }), load: async () => true }) })
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
  globalThis.$fetch = (url, input) => url.endsWith('/capture-challenge') ? Promise.resolve({data:{event_id:'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'}}) : (options = input, new Promise(resolve => { finish = resolve }))
  const page = await kioskSetup()
  try {
    await mounted(); page.camera.value = { capture: () => '/9j/AA==', stop() {} }
    page.cameraChanged(true)
    await flush()
    assert.equal(options.headers['X-CSRF-Token'], 'csrf')
    assert.equal(options.credentials, 'include')
    assert.equal(options.body.event_id, 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa')
    page.stopCamera()
    finish({ data: { decision: 'accepted', recognition_enabled: true, attendance: { outcome: 'recorded' }, faces: [{ box: { x: 0, y: 0, width: 0.5, height: 0.5 }, label: 'Old identity', student_id: 'old' }] } })
    await flush(); assert.deepEqual(page.faces.value, []); assert.equal(page.cameraActive.value, false)
  }
  finally { unmounted(); globalThis.$fetch = originalFetch }
})

test('pending frames retain the live box, then clear it on disappearance without discarding the last attendance', async () => {
  context(async () => ({data: session}))
  const originalFetch=globalThis.$fetch
  let finish
  globalThis.$fetch=(url)=>url.endsWith('/capture-challenge') ? Promise.resolve({data:{event_id:'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'}}) : new Promise(resolve=>{finish=resolve})
  const page=await kioskSetup()
  try {
    await mounted()
    page.camera.value={capture:()=>'/9j/AA==',stop(){}}
    const known={decision:'accepted',recognition_enabled:true,faces:[{box:{x:0,y:0,width:0.5,height:0.5},student_id:'student',label:'Siswa A',similarity:0.89}],attendance:{outcome:'recorded',mode:'check_in',record:{id:'record',status:'present',check_in_at:'2026-10-15T00:00:00Z',check_out_at:null}}}
    page.showResult(known)
    page.cameraChanged(true)
    assert.equal(page.faces.value[0].label,'Siswa A')
    await flush()
    finish({data:{decision:'no_face',recognition_enabled:false,faces:[]}})
    await flush()
    assert.deepEqual(page.faces.value,[])
    assert.equal(page.lastResult.value.name,'Siswa A')
    assert.equal(page.recent.value.length,1)
    page.showResult(known); assert.equal(page.recent.value.length,1)
    page.clock.value=Date.parse('2026-10-15T08:00:00Z')
    page.kiosk.value.checkout_at='2026-10-15T08:00:00Z'
    assert.equal(page.departure.value,true)
    assert.equal(page.formatTime('2026-10-15T00:00:00Z'),'07.00')
  } finally {unmounted(); globalThis.$fetch=originalFetch}
})

test('kiosk confirms committed attendance once per pupil, supports the next pupil and distinguishes departure', async () => {
 context(async()=>({data:session}))
 const page=await kioskSetup()
 const match=(id,name,outcome='recorded',departed=false)=>({decision:'accepted',recognition_enabled:true,faces:[{box:{x:0,y:0,width:0.5,height:0.5},student_id:id,label:name,similarity:0.9}],attendance:{outcome,mode:departed?'check_out':'check_in',record:{id,status:'present',check_in_at:'2026-10-15T00:00:00Z',check_out_at:departed?'2026-10-15T08:00:00Z':null}}})
 try {
  await mounted();page.showResult(match('a','Siswa A'))
  assert.equal(page.notice.value.name,'Siswa A');assert.equal(page.notice.value.saved,true)
  assert.match(page.notice.value.title,/masuk/i)
  const first=page.notice.value
  const repeated=match('a','Siswa A','already_recorded');repeated.faces[0].similarity=0.71
  page.showResult(repeated)
  assert.equal(page.lastResult.value.similarity,0.9,'repeat scan replaced the first verified score')
  assert.equal(page.notice.value,first,'continuous frames repeated the notification')
  const nextPupil=match('b','Siswa B');nextPupil.faces[0].similarity=0.94
  page.showResult(nextPupil);assert.equal(page.notice.value.name,'Siswa B')
  assert.equal(page.lastResult.value.similarity,0.94,'next pupil reused a different pupils score')
  page.showResult(match('a','Siswa A','already_recorded'))
  const departureMatch=match('a','Siswa A','checked_out',true);departureMatch.faces[0].similarity=0.83
  page.showResult(departureMatch);assert.match(page.notice.value.title,/pulang/i)
  assert.equal(page.lastResult.value.similarity,0.83,'departure reused the arrival score')
  assert.equal(page.recent.value.length,3)
  page.showResult({...match('c','Siswa C'),attendance:{outcome:'check_in_required',mode:'check_out',record:null}})
  assert.equal(page.notice.value.saved,false);assert.match(page.notice.value.title,/belum/i)
  assert.equal(page.recent.value.length,3)
  const originalNow=Date.now;let now=originalNow()
  try {
    Date.now=()=>now
    page.showResult(match('a','Siswa A','already_checked_out',true))
    const prior=page.notice.value
    page.showResult({decision:'no_face',faces:[],recognition_enabled:false})
    now+=1600;page.showResult({decision:'no_face',faces:[],recognition_enabled:false})
    page.showResult(match('a','Siswa A','already_checked_out',true))
    assert.notEqual(page.notice.value,prior,'returning pupil did not receive confirmation')
  } finally {Date.now=originalNow}
  for(let n=0;n<100;n++) page.showResult(match(`pupil-${n}`,`Siswa ${n}`))
  assert.equal(page.recent.value.length,5);assert.equal(page.notice.value.name,'Siswa 99')
  page.stopCamera();assert.equal(page.notice.value,null)
 } finally {unmounted()}
})


test('stopped camera never captures or recognizes after a late capture permit', async () => {
 context(async()=>({data:session}))
 const originalFetch=globalThis.$fetch
 let permit, captures=0
 const requests=[]
 globalThis.$fetch=(url)=>{requests.push(url);return new Promise(resolve=>{permit=resolve})}
 const page=await kioskSetup()
 try{
  await mounted();page.camera.value={capture:()=>{captures++;return '/9j/AA=='},stop(){}}
  page.cameraChanged(true)
  assert.equal(captures,0)
  assert.ok(requests[0].endsWith('/capture-challenge'))
  page.stopCamera();permit({data:{event_id:'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'}})
  await flush()
  assert.equal(captures,0);assert.equal(requests.length,1);assert.equal(page.cameraActive.value,false)
 }finally{unmounted();globalThis.$fetch=originalFetch}
})

test('paired kiosk waits between sessions and follows the next session without a pairing code', async () => {
  context(async () => ({ data: { ...session, session_id: 'old', session_ready: true } }))
  const originalFetch = globalThis.$fetch
  let response = { ...session, session_id: null, session_ready: false, connection_status: 'waiting_session' }
  globalThis.$fetch = async () => ({ data: response })
  const page = await kioskSetup()
  try {
    await mounted(); page.cameraActive.value = true
    await page.refreshConnection()
    assert.ok(page.kiosk.value); assert.equal(page.sessionReady.value, false); assert.equal(page.cameraActive.value, false)
    response = { ...session, session_id: 'next', session_ready: true, connection_status: 'ready' }
    await page.refreshConnection()
    assert.equal(page.kiosk.value.session_id, 'next'); assert.equal(page.sessionReady.value, true)
    assert.equal(page.pairingToken.value, '')
  } finally { unmounted(); globalThis.$fetch = originalFetch }
})

test('opening kiosk offline restores the remembered device when connectivity returns', async () => {
  let available = false
  context(async path => {
    if (path === '/kiosk/me') {
      if (!available) throw new Error('offline')
      return { data: session }
    }
    return { data: [] }
  })
  const page = await kioskSetup()
  try {
    await mounted(); assert.equal(page.kiosk.value, null)
    available = true; window.dispatchEvent(new Event('online')); await flush()
    assert.equal(page.kiosk.value.device_name, 'Gate')
    assert.equal(page.pairingToken.value, '')
  } finally { unmounted() }
})
