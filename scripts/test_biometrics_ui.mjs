import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { captureJpeg, faceOverlays } from '../apps/web/app/lib/biometrics.ts'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { ref } = requireWeb('vue')
const box = { x: 0.2, y: 0.3, width: 0.2, height: 0.4 }
const result = { decision: 'accepted', recognition_enabled: true, faces: [{ box, student_id: 'student', label: 'Siswa A' }], attendance: { outcome: 'recorded' } }

test('face names require accepted server identity and committed attendance', () => {
  assert.equal(faceOverlays(result)[0].label, 'Siswa A')
  for (const change of [{ decision: 'unknown' }, { recognition_enabled: false }, { attendance: undefined }, { attendance: { outcome: 'pending' } }]) {
    assert.equal(faceOverlays({ ...result, ...change })[0].label, 'Belum dikenali')
    assert.equal(faceOverlays({ ...result, ...change })[0].accepted, false)
  }
  assert.equal(faceOverlays({ ...result, faces: [{ box, label: 'Untrusted candidate' }] })[0].accepted, false)
})

test('invalid or out-of-frame face boxes are not rendered', () => {
  for (const change of [{ x: NaN }, { width: 0 }, { y: -1 }, { x: 0.9 }, { height: Infinity }]) {
    assert.deepEqual(faceOverlays({ ...result, faces: [{ ...result.faces[0], box: { ...box, ...change } }] }), [])
  }
})

test('capture resizes without cropping, strips data URL, clears canvas and bounds payload', () => {
  const canvas = { width: 0, height: 0, getContext: () => ({ drawImage: (...args) => { assert.deepEqual(args.slice(1), [0, 0, 960, 540]) } }), toDataURL: () => 'data:image/jpeg;base64,/9j/AA==' }
  globalThis.document = { createElement: () => canvas }
  assert.equal(captureJpeg({ readyState: 2, videoWidth: 1920, videoHeight: 1080 }), '/9j/AA==')
  assert.equal(canvas.width, 0); assert.equal(canvas.height, 0)
  assert.throws(() => captureJpeg({ readyState: 0 }), /belum siap/)
  canvas.toDataURL = () => 'data:image/jpeg;base64,' + 'a'.repeat(384 * 1024 + 1)
  assert.throws(() => captureJpeg({ readyState: 2, videoWidth: 1920, videoHeight: 1080 }), /terlalu besar/)
  assert.equal(canvas.width, 0)
})

const cameraStorage = new Map()
async function cameraSetup(getUserMedia, devices = []) {
  let mounted, unmounted
  Object.assign(globalThis, { ref, useId: () => 'camera-select', onMounted: fn => { mounted = fn }, onBeforeUnmount: fn => { unmounted = fn } })
  globalThis.document = Object.assign(new EventTarget(), { hidden: false })
  globalThis.window = Object.assign(new EventTarget(), { localStorage: { getItem: key => cameraStorage.get(key) ?? null, setItem: (key, value) => cameraStorage.set(key, value) } })
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { onLine: true, mediaDevices: { getUserMedia, enumerateDevices: async () => devices } } })
  const directory = resolve('.tools/camera-regression')
  mkdirSync(directory, { recursive: true })
  try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') } catch (error) { if (error.code !== 'EEXIST') throw error }
  const { descriptor } = parse(readFileSync('apps/web/app/components/FaceCamera.vue', 'utf8'))
  const compiled = compileScript(descriptor, { id: 'camera-regression' })
  const file = directory + '/camera.ts'
  writeFileSync(file, compiled.content.replaceAll('../lib/biometrics.ts', pathToFileURL(resolve('apps/web/app/lib/biometrics.ts')).href))
  const module = await import(pathToFileURL(file).href + '?' + Math.random())
  const events = []
  const page = module.default.setup({ faces: [] }, { expose() {}, emit: (...args) => events.push(args) })
  await mounted()
  return { page, unmounted, events }
}

test('late permission result after unmount stops every track', async () => {
  let finish, stops = 0
  const { page, unmounted, events } = await cameraSetup(() => new Promise(resolve => { finish = resolve }))
  const pending = page.start(); unmounted()
  finish({ getTracks: () => [{ stop: () => stops++ }] })
  await pending
  assert.equal(stops, 1); assert.equal(page.active.value, false)
  assert.equal(events.some(([name, value]) => name === 'active' && value), false)
})

test('camera stops when hidden/offline and ignores overlapping start requests', async () => {
  let calls = 0, stops = 0
  const track = new EventTarget(); track.stop = () => stops++
  const { page, unmounted } = await cameraSetup(async () => { calls++; return { getTracks: () => [track], getVideoTracks: () => [track] } })
  page.video.value = { srcObject: null, videoWidth: 640, videoHeight: 480, play: async () => {} }
  try {
    await Promise.all([page.start(), page.start()])
    assert.equal(calls, 1); assert.equal(page.active.value, true); assert.equal(page.ratio.value, 640 / 480)
    document.hidden = true; document.dispatchEvent(new Event('visibilitychange'))
    assert.equal(stops, 1); assert.equal(page.video.value.srcObject, null)
    await page.start(); assert.equal(calls, 1)
    document.hidden = false; await page.start()
    window.dispatchEvent(new Event('offline')); assert.equal(stops, 2); assert.equal(page.active.value, false)
  }
  finally { unmounted() }
})
process.on('exit', () => rmSync(resolve('.tools/camera-regression'), { recursive: true, force: true }))

 test('camera prefers Fantech, respects explicit selection and remembers it for enrollment and kiosk', async () => {
  const devices = [{ kind: 'videoinput', deviceId: 'internal', label: 'Internal camera' }, {kind: 'videoinput', deviceId: 'fantech', label: 'Fantech Luminous C30'}]
  let input
  const track = new EventTarget(); track.stop = () => {}; track.getSettings = () => ({deviceId: 'internal'})
  const media = async constraints => {input=constraints;return {getTracks:()=>[track],getVideoTracks:()=>[track]}}
  const {page,unmounted}=await cameraSetup(media,devices)
  page.video.value = {srcObject:null,videoWidth:640,videoHeight:480,play:async()=>{}}
  try {
   assert.equal(page.selectedCamera.value,'fantech')
   page.selectedCamera.value='internal'
   await page.start()
   assert.deepEqual(input.video.deviceId,{exact:'internal'})
   assert.equal(input.audio,false)
  } finally {unmounted()}
  const next=await cameraSetup(media,devices)
  try {assert.equal(next.page.selectedCamera.value,'internal')} finally {next.unmounted();cameraStorage.clear()}
 })

test('live similarity remains a bounded cosine score and scheduled departure is identified', () => {
  const matched = faceOverlays({ ...result, faces: [{ ...result.faces[0], similarity: 0.89 }], attendance: { outcome: 'checked_out' } })[0]
  assert.equal(matched.label, 'Siswa A'); assert.equal(matched.similarity, 0.89)
  for (const similarity of [NaN, Infinity, 1.01, -1.01, '0.9']) {
    assert.equal(faceOverlays({ ...result, faces: [{ ...result.faces[0], similarity }] })[0].similarity, undefined)
  }
})

test('attendance feedback separates arrival, departure and protected or missing records', async () => {
  const { attendanceFeedback } = await import('../apps/web/app/lib/biometrics.ts')
  const record = { status: 'late', check_in_at: '2026-10-15T00:40:00Z', check_out_at: null }
  assert.equal(attendanceFeedback({ outcome: 'recorded', record }).status, 'Terlambat')
  assert.equal(attendanceFeedback({ outcome: 'already_recorded', record }).message, 'Kehadiran sudah tercatat.')
  assert.equal(attendanceFeedback({ outcome: 'checked_out', record: { ...record, check_out_at: '2026-10-15T08:00:00Z' } }).status, 'Sudah pulang')
  for (const outcome of ['check_in_required', 'correction_pending', 'status_locked']) {
    assert.equal(attendanceFeedback({ outcome, record: null }).saved, false)
  }
})


test('a fresh verified track may keep its name during PAD recheck without showing attendance success', () => {
 const pending={...result,decision:'verification_required',recognition_enabled:false,identity_continuous:true,attendance:undefined}
 const face=faceOverlays(pending)[0]
 assert.equal(face.label,'Siswa A');assert.equal(face.accepted,false);assert.equal(face.tone,'warning')
 assert.equal(faceOverlays({...pending,identity_continuous:false})[0].label,'Belum dikenali')
 assert.equal(faceOverlays({...pending,decision:'unknown'})[0].label,'Belum dikenali')
})
