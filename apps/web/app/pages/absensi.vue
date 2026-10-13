<script setup lang="ts">
import { Camera, CameraOff, ShieldCheck } from '@lucide/vue'
import { pairingSchema } from '../lib/validation/consent-devices.ts'
import { useFormValidation } from '../composables/useFormValidation.ts'
import { faceOverlays } from '../lib/biometrics.ts'
import type { FaceOverlay, Recognition } from '~/types/biometrics'
import type { Result } from '~/types/auth'
import type { AttendanceSession } from '~/types/attendance'
definePageMeta({ layout: false })
useHead({ title: 'Absensi sekolah', meta: [{ name: 'robots', content: 'noindex, nofollow' }] })
const request = useApi()
const auth = useAuth()
const { options } = useMasterData()
interface Kiosk {
  device_name: string
  session_name: string
  expires_at: string
  csrf_token: string
  recognition_enabled: boolean
}
const kiosk = ref<Kiosk | null>(null)
const sessions = ref<AttendanceSession[]>([])
const pairingToken = ref('')
const sessionId = ref('')
const busy = ref(false)
const message = ref('')
const cameraActive = ref(false)
const { errors, validate, clear } = useFormValidation(
  () => pairingSchema,
  () => ({ pairing_token: pairingToken.value, session_id: sessionId.value }),
)
const camera = ref<{ start: () => Promise<void>; stop: () => void; capture: () => string } | null>(
  null,
)
const cameraBusy = ref(false)
const faces = ref<FaceOverlay[]>([])
const scanMessage = ref('')
const config = useRuntimeConfig()
let heartbeat: ReturnType<typeof setInterval> | undefined
let expiry: ReturnType<typeof setTimeout> | undefined
let scanTimer: ReturnType<typeof setTimeout> | undefined
let scanAbort: AbortController | undefined
let scanGeneration = 0
let trackId = ''
let disposed = false
let initialized = false
let discovering = false

function stopCamera() {
  scanGeneration++
  clearTimeout(scanTimer)
  scanAbort?.abort()
  camera.value?.stop()
  cameraActive.value = false
  faces.value = []
  scanMessage.value = ''
}
function stop(reason = '') {
  stopCamera()
  clearInterval(heartbeat)
  clearTimeout(expiry)
  kiosk.value = null
  if (reason) message.value = reason
}
function hidden() {
  if (document.hidden) stopCamera()
}
function offline() {
  stop(
    'Koneksi terputus. Gunakan prosedur absensi manual; tidak ada kehadiran otomatis yang dicatat.',
  )
}
async function startCamera() {
  if (!kiosk.value || !navigator.onLine || document.hidden) return
  await camera.value?.start()
}
function cameraChanged(active: boolean) {
  cameraActive.value = active
  if (!active) {
    scanGeneration++
    clearTimeout(scanTimer)
    scanAbort?.abort()
    faces.value = []
    return
  }
  trackId = crypto.randomUUID()
  const generation = ++scanGeneration
  void scan(generation)
}
async function scan(generation: number) {
  if (
    generation !== scanGeneration ||
    disposed ||
    !kiosk.value ||
    !cameraActive.value ||
    document.hidden ||
    !navigator.onLine
  )
    return
  scanAbort = new AbortController()
  faces.value = []
  try {
    const image = camera.value?.capture()
    if (!image) return
    const result = await $fetch<Result<Recognition>>(`${config.public.apiBase}/kiosk/recognize`, {
      method: 'POST',
      credentials: 'include',
      retry: 0,
      timeout: 8000,
      signal: scanAbort.signal,
      headers: { 'X-CSRF-Token': kiosk.value.csrf_token },
      body: {
        image,
        event_id: crypto.randomUUID(),
        track_id: trackId,
        captured_at: new Date().toISOString(),
      },
    })
    if (generation !== scanGeneration || disposed) return
    faces.value = faceOverlays(result.data)
    scanMessage.value =
      result.data.decision === 'accepted' && faces.value.some((face) => face.accepted)
        ? result.data.attendance?.outcome === 'recorded'
          ? 'Kehadiran tersimpan.'
          : 'Kehadiran sudah tercatat.'
        : result.data.faces.length
          ? 'Wajah terdeteksi · identitas belum diterima.'
          : 'Posisikan satu wajah di depan kamera.'
  } catch (cause) {
    if (generation !== scanGeneration || disposed) return
    scanMessage.value = apiMessage(cause)
    if ([401, 403, 409, 410].includes((cause as { status?: number }).status ?? 0)) {
      stop(
        'Sesi perangkat tidak lagi dapat digunakan. Pasangkan ulang atau gunakan absensi manual.',
      )
      return
    }
  } finally {
    if (generation === scanGeneration && !disposed && cameraActive.value)
      scanTimer = setTimeout(() => void scan(generation), 1000)
  }
}
function arm(data: Kiosk) {
  if (disposed) return
  stop()
  kiosk.value = data
  const remaining = new Date(data.expires_at).getTime() - Date.now()
  if (remaining <= 0) {
    stop('Sesi perangkat kedaluwarsa. Pasangkan ulang.')
    return
  }
  expiry = setTimeout(() => stop('Sesi perangkat kedaluwarsa. Pasangkan ulang.'), remaining)
  heartbeat = setInterval(async () => {
    if (!kiosk.value || disposed) return
    const current = kiosk.value
    try {
      await $fetch(`${config.public.apiBase}/kiosk/heartbeat`, {
        method: 'POST',
        credentials: 'include',
        retry: 0,
        timeout: 8000,
        headers: { 'X-CSRF-Token': current.csrf_token },
      })
    } catch {
      if (!disposed && kiosk.value === current)
        stop(
          'Perangkat/sesi tidak aktif atau koneksi bermasalah. Pasangkan ulang setelah layanan pulih.',
        )
    }
  }, 15000)
}
async function pair() {
  if (busy.value || !validate()) return
  busy.value = true
  message.value = ''
  try {
    const result = await request<Result<Kiosk>>('/kiosk/session', {
      method: 'POST',
      body: { pairing_token: pairingToken.value.trim(), session_id: sessionId.value },
    })
    pairingToken.value = ''
    clear()
    auth.session.value = null
    arm(result.data)
  } catch (e) {
    message.value = apiMessage(e)
  } finally {
    busy.value = false
  }
}
async function discover() {
  if (disposed || discovering) return
  discovering = true
  sessions.value = []
  try {
    await auth.load()
    if (disposed) return
    if (auth.session.value?.permissions.includes('devices.health')) {
      const data = await options<AttendanceSession>('/kiosk/session-options')
      if (!disposed) sessions.value = data
    } else message.value = 'Operator perlu masuk untuk memasangkan perangkat.'
  } catch {
    if (!disposed) message.value = 'Daftar sesi belum tersedia. Periksa koneksi dan akses operator.'
  } finally {
    discovering = false
  }
}
watch(kiosk, (value) => {
  if (!value && initialized && !disposed) void discover()
})
onMounted(async () => {
  document.addEventListener('visibilitychange', hidden)
  window.addEventListener('offline', offline)
  try {
    const result = await request<Result<Kiosk>>('/kiosk/me')
    if (!disposed) arm(result.data)
  } catch {
    /* An unpaired device starts at the pairing form. */
  }
  if (disposed) return
  initialized = true
  if (!kiosk.value) await discover()
})
onBeforeUnmount(() => {
  disposed = true
  stop()
  document.removeEventListener('visibilitychange', hidden)
  window.removeEventListener('offline', offline)
})
</script>
<template>
  <main class="min-h-svh bg-muted/40 px-4 py-6 sm:px-8 sm:py-10">
    <div class="mx-auto flex max-w-5xl flex-col gap-6">
      <header class="flex items-center justify-between gap-4">
        <div>
          <p class="text-sm font-medium text-muted-foreground">Absensi sekolah</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-tight">
            {{ kiosk ? 'Kiosk absensi' : 'Pasangkan perangkat' }}
          </h1>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <ModeToggle />
          <Badge :variant="kiosk ? 'success' : 'secondary'">
            <ShieldCheck class="mr-1 size-4" />
            {{ kiosk ? 'Perangkat terdaftar' : 'Belum terhubung' }}
          </Badge>
        </div>
      </header>
      <Alert
        v-if="message"
        variant="destructive"
        role="alert"
      >
        <AlertDescription>{{ message }}</AlertDescription>
      </Alert>
      <Card
        v-if="!kiosk"
        class="mx-auto w-full max-w-lg"
      >
        <CardHeader>
          <CardTitle>Pairing oleh operator</CardTitle>
          <CardDescription>
            Masuk sebagai operator, pilih sesi terbuka dan masukkan token yang diterbitkan admin.
          </CardDescription>
          <CardAction>
            <Button
              type="submit"
              form="kiosk-pairing"
              :disabled="busy"
            >
              <Spinner v-if="busy" />
              Pasangkan
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <form
            id="kiosk-pairing"
            class="space-y-6"
            novalidate
            @submit.prevent="pair"
          >
            <FieldGroup>
              <Field :data-invalid="!!errors.pairing_token">
                <FieldLabel for="kiosk-token">Token sekali pakai</FieldLabel>
                <Input
                  id="kiosk-token"
                  v-model="pairingToken"
                  :aria-invalid="!!errors.pairing_token"
                  :aria-describedby="errors.pairing_token ? 'kiosk-token-error' : undefined"
                  required
                  maxlength="64"
                  autocomplete="off"
                  :disabled="busy"
                />
                <FieldError
                  v-if="errors.pairing_token"
                  id="kiosk-token-error"
                >
                  {{ errors.pairing_token }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.session_id">
                <FieldLabel for="kiosk-session">Sesi absensi</FieldLabel>
                <Select
                  v-model="sessionId"
                  :disabled="busy"
                >
                  <SelectTrigger
                    id="kiosk-session"
                    :aria-invalid="!!errors.session_id"
                    :aria-describedby="errors.session_id ? 'kiosk-session-error' : undefined"
                    class="w-full"
                  >
                    <SelectValue placeholder="Pilih sesi terbuka" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="item in sessions"
                      :key="item.id"
                      :value="item.id"
                    >
                      {{ item.name }} · {{ item.class_name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FieldError
                  v-if="errors.session_id"
                  id="kiosk-session-error"
                >
                  {{ errors.session_id }}
                </FieldError>
              </Field>
            </FieldGroup>
            <Button
              v-if="!auth.session.value"
              as-child
              variant="outline"
              class="w-full"
            >
              <NuxtLink to="/login">Masuk sebagai operator</NuxtLink>
            </Button>
          </form>
        </CardContent>
      </Card>
      <template v-else>
        <Card>
          <CardHeader
            class="has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]"
          >
            <CardTitle>{{ kiosk.device_name }}</CardTitle>
            <CardDescription>
              {{ kiosk.session_name }} · sesi perangkat berakhir
              {{ new Date(kiosk.expires_at).toLocaleTimeString('id-ID') }}
            </CardDescription>
            <CardAction
              class="col-start-1 row-start-3 row-span-1 justify-self-end sm:col-start-2 sm:row-start-1 sm:row-span-2"
            >
              <Button
                v-if="!cameraActive"
                :disabled="busy || cameraBusy"
                @click="startCamera"
              >
                <Camera />
                Aktifkan kamera
              </Button>
              <Button
                v-else
                variant="outline"
                @click="stopCamera"
              >
                <CameraOff />
                Hentikan kamera
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent class="space-y-4">
            <FaceCamera
              ref="camera"
              :faces="faces"
              @active="cameraChanged"
              @busy="cameraBusy = $event"
            />
            <p
              role="status"
              aria-live="polite"
              class="text-sm text-muted-foreground"
            >
              {{ cameraActive ? scanMessage || 'Memeriksa frame kamera…' : 'Kamera berhenti' }}
            </p>
          </CardContent>
        </Card>
        <Alert>
          <AlertTitle>Akurasi & liveness belum dievaluasi</AlertTitle>
          <AlertDescription>
            Frame kamera diproses untuk deteksi tanpa menyimpan foto. Kotak menampilkan nama hanya
            setelah identitas diterima dan kehadiran terkonfirmasi. Absensi otomatis masih menunggu
            evaluasi; gunakan absensi manual melalui petugas.
          </AlertDescription>
        </Alert>
      </template>
    </div>
  </main>
</template>
