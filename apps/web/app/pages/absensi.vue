<script setup lang="ts">
import { toast } from 'vue-sonner'
import { Camera, CameraOff, ShieldCheck, Clock, LogIn, LogOut, CircleCheck } from '@lucide/vue'
import { pairingSchema } from '../lib/validation/biometrics-devices.ts'
import { useFormValidation } from '../composables/useFormValidation.ts'
import { faceOverlays, attendanceFeedback } from '../lib/biometrics.ts'
import type { FaceOverlay, Recognition } from '~/types/biometrics'
import type { Result } from '~/types/auth'
import type { AttendanceSession } from '~/types/attendance'
definePageMeta({ layout: false })
useHead({ title: 'Absensi sekolah', meta: [{ name: 'robots', content: 'noindex, nofollow' }] })
const request = useApi()
const auth = useAuth()
const { options } = useMasterData()
interface Kiosk {
  session_id?: string | null
  session_ready?: boolean
  connection_status?: 'ready' | 'waiting_session' | 'multiple_sessions'
  device_name: string
  session_name: string
  expires_at: string
  csrf_token: string
  recognition_enabled: boolean
  timezone: string
  class_name: string
  starts_at: string
  late_after: string
  checkout_at: string | null
  closes_at: string
  server_time: string
}
const kiosk = ref<Kiosk | null>(null)
const sessionReady = computed(() => !!kiosk.value && kiosk.value.session_ready !== false)
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
const clock = ref(Date.now())
let clockOffset = 0
let clockTimer: ReturnType<typeof setInterval> | undefined
let overlayExpiry: ReturnType<typeof setTimeout> | undefined
const departure = computed(
  () => !!kiosk.value?.checkout_at && clock.value >= Date.parse(kiosk.value.checkout_at),
)
const lastResult = ref<{
  name: string
  referencePhoto?: string
  similarity?: number
  attendance: NonNullable<Recognition['attendance']>
} | null>(null)
const feedback = computed(() =>
  lastResult.value ? attendanceFeedback(lastResult.value.attendance) : null,
)
const recent = ref<{ key: string; name: string; time: string; departed: boolean }[]>([])
interface AttendanceNotice {
  name: string
  title: string
  status: string
  message: string
  time: string | null
  saved: boolean
}
const notice = ref<AttendanceNotice | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | undefined
let notificationKey = ''
let absentSince: number | null = null
function announce(key: string, next: AttendanceNotice) {
  if (key === notificationKey) return
  notificationKey = key
  clearTimeout(noticeTimer)
  notice.value = next
  const description = `${next.name} · ${next.status}${next.time ? ` · ${formatTime(next.time, true)}` : ''}`
  if (next.saved)
    toast.success(next.title, {
      id: 'kiosk-attendance',
      description,
      duration: 5000,
    })
  else
    toast.warning(next.title, {
      id: 'kiosk-attendance',
      description,
      duration: 5000,
    })
  noticeTimer = setTimeout(() => {
    notice.value = null
  }, 5000)
}

function formatTime(value: string | number | null | undefined, seconds = false) {
  if (value == null) return '—'
  return new Intl.DateTimeFormat('id-ID', {
    timeZone: kiosk.value?.timezone ?? 'Asia/Jakarta',
    hour: '2-digit',
    minute: '2-digit',
    ...(seconds ? { second: '2-digit' } : {}),
  }).format(new Date(value))
}
const schoolDate = computed(() =>
  new Intl.DateTimeFormat('id-ID', {
    timeZone: kiosk.value?.timezone ?? 'Asia/Jakarta',
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(clock.value),
)
function syncClock(serverTime?: string) {
  if (serverTime && Number.isFinite(Date.parse(serverTime)))
    clockOffset = Date.parse(serverTime) - Date.now()
  clock.value = Date.now() + clockOffset
}
function showResult(result: Recognition) {
  faces.value = faceOverlays(result)
  if (result.decision === 'no_face') {
    absentSince ??= Date.now()
    if (Date.now() - absentSince >= 1500) notificationKey = ''
  } else absentSince = null
  syncClock(result.server_time)
  const identified = faces.value.find((face) => face.accepted)
  if (identified && result.attendance) {
    const previous = lastResult.value
    const sameRecord =
      previous &&
      result.attendance.record &&
      previous.attendance.record?.id === result.attendance.record.id &&
      previous.attendance.mode === result.attendance.mode
    lastResult.value = {
      name: identified.label,
      referencePhoto: result.reference_photo,
      similarity: sameRecord
        ? (previous.similarity ?? identified.similarity)
        : identified.similarity,
      attendance: result.attendance,
    }
    const saved = attendanceFeedback(result.attendance)
    const record = result.attendance.record
    if (saved.saved && record) {
      const departed = !!record.check_out_at
      const key = `${record.id}:${departed ? 'out' : 'in'}`
      announce(key, {
        name: identified.label,
        title: departed
          ? result.attendance.outcome === 'checked_out'
            ? 'Absen pulang berhasil'
            : 'Sudah absen pulang'
          : result.attendance.outcome === 'recorded'
            ? 'Absen masuk berhasil'
            : 'Sudah absen masuk',
        status: saved.status,
        message: 'Selesai. Silakan beri tempat untuk siswa berikutnya.',
        time: departed ? record.check_out_at : record.check_in_at,
        saved: true,
      })
      if (!recent.value.some((item) => item.key === key))
        recent.value = [
          {
            key,
            name: identified.label,
            time: (departed ? record.check_out_at : record.check_in_at)!,
            departed,
          },
          ...recent.value,
        ].slice(0, 5)
    }
    if (!saved.saved)
      announce(`${result.faces[0]?.student_id}:${result.attendance.outcome}`, {
        name: identified.label,
        title: 'Absensi belum dapat dicatat',
        status: saved.status,
        message: saved.message,
        time: null,
        saved: false,
      })
    scanMessage.value = saved.message
  } else {
    const guidance: Record<string, string> = {
      PROCESSING: 'Tahan posisi sebentar. Memastikan wajah cocok…',
      VERIFYING:
        'Wajah masih cocok. Perbaiki posisi dan pencahayaan; pencatatan menunggu verifikasi selesai.',
      LOW_QUALITY: 'Hadapkan wajah ke kamera dan perbaiki pencahayaan.',
      MULTIPLE_FACES: 'Bergantian, satu siswa di depan kamera.',
      UNKNOWN: 'Wajah belum dikenali. Hadapkan wajah ke kamera atau hubungi petugas.',
      NO_FACE: 'Posisikan satu wajah di depan kamera.',
    }
    scanMessage.value =
      result.reason === 'PAD_REJECTED' && result.engine_status !== 'VERIFYING'
        ? 'Wajah belum dapat diverifikasi. Hadapkan wajah langsung ke kamera.'
        : (guidance[result.engine_status ?? ''] ??
          (result.faces.length ? 'Memeriksa wajah…' : 'Posisikan satu wajah di depan kamera.'))
  }
}
const config = useRuntimeConfig()
let heartbeat: ReturnType<typeof setInterval> | undefined
let scanTimer: ReturnType<typeof setTimeout> | undefined
let scanAbort: AbortController | undefined
let scanGeneration = 0
let trackId = ''
let disposed = false
let initialized = false
let discovering = false

function stopCamera() {
  scanGeneration++
  clearTimeout(noticeTimer)
  notice.value = null
  notificationKey = ''
  absentSince = null
  toast.dismiss('kiosk-attendance')
  clearTimeout(scanTimer)
  clearTimeout(overlayExpiry)
  scanAbort?.abort()
  camera.value?.stop()
  cameraActive.value = false
  faces.value = []
  scanMessage.value = ''
}
function stop(reason = '') {
  stopCamera()
  clearInterval(heartbeat)
  kiosk.value = null
  if (reason) message.value = reason
}
function hidden() {
  if (document.hidden) stopCamera()
}
function offline() {
  stopCamera()
  message.value =
    'Koneksi terputus. Perangkat akan mencoba terhubung kembali. Gunakan absensi manual sementara.'
}
async function startCamera() {
  if (!sessionReady.value || !navigator.onLine || document.hidden) return
  await camera.value?.start()
}
function cameraChanged(active: boolean) {
  cameraActive.value = active
  if (!active) {
    scanGeneration++
    clearTimeout(scanTimer)
    clearTimeout(overlayExpiry)
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
    !sessionReady.value ||
    !kiosk.value ||
    !cameraActive.value ||
    document.hidden ||
    !navigator.onLine
  )
    return
  scanAbort = new AbortController()
  // Retain the current box only while a fresh frame is pending, with a bounded lifetime.
  const pendingExpiry = setTimeout(() => {
    if (generation === scanGeneration) {
      faces.value = []
      scanMessage.value = 'Memeriksa kamera… Tunggu sebentar.'
    }
  }, 1200)
  overlayExpiry = pendingExpiry
  try {
    const permit = await $fetch<Result<{ event_id: string }>>(
      `${config.public.apiBase}/kiosk/capture-challenge`,
      {
        method: 'POST',
        credentials: 'include',
        retry: 0,
        timeout: 8000,
        signal: scanAbort.signal,
        headers: { 'X-CSRF-Token': kiosk.value.csrf_token },
      },
    )
    if (generation !== scanGeneration || disposed || !kiosk.value || !cameraActive.value) return
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
        event_id: permit.data.event_id,
        track_id: trackId,
        captured_at: new Date().toISOString(),
      },
    })
    if (generation !== scanGeneration || disposed) return
    showResult(result.data)
  } catch (cause) {
    if (generation !== scanGeneration || disposed) return
    faces.value = []
    scanMessage.value = apiMessage(cause)
    if (
      (cause as { data?: { error?: { code?: string } } }).data?.error?.code === 'CAPTURE_REPLAY' ||
      (cause as { data?: { error?: { code?: string } } }).data?.error?.code === 'RECOGNITION_STALE'
    )
      return
    if (
      (cause as { data?: { error?: { code?: string } } }).data?.error?.code ===
      'SESSION_UNAVAILABLE'
    ) {
      stopCamera()
      await refreshConnection()
      return
    }
    if ([401, 403, 409, 410].includes((cause as { status?: number }).status ?? 0)) {
      stop(
        'Sesi perangkat tidak lagi dapat digunakan. Pasangkan ulang atau gunakan absensi manual.',
      )
      return
    }
  } finally {
    clearTimeout(pendingExpiry)
    if (generation === scanGeneration && !disposed && cameraActive.value)
      scanTimer = setTimeout(() => void scan(generation), 250)
  }
}
function arm(data: Kiosk) {
  if (disposed) return
  stop()
  kiosk.value = data
  lastResult.value = null
  recent.value = []
  syncClock(data.server_time)
  if (Date.parse(data.expires_at) <= clock.value) {
    stop('Koneksi perangkat sudah berakhir. Hubungkan kembali dengan kode baru.')
    return
  }
  heartbeat = setInterval(() => void refreshConnection(), 15000)
}
async function refreshConnection() {
  if (!kiosk.value || disposed) return
  const current = kiosk.value
  try {
    const result = await $fetch<Result<Kiosk>>(`${config.public.apiBase}/kiosk/heartbeat`, {
      method: 'POST',
      credentials: 'include',
      retry: 0,
      timeout: 8000,
      headers: { 'X-CSRF-Token': current.csrf_token },
    })
    if (disposed || kiosk.value !== current) return
    const next = result.data
    if (next.session_id !== current.session_id || next.session_ready !== current.session_ready) {
      stopCamera()
      lastResult.value = null
      recent.value = []
    }
    kiosk.value = next
    syncClock(next.server_time)
    message.value = ''
  } catch (cause) {
    if (disposed || kiosk.value !== current) return
    if ([401, 403, 410].includes((cause as { status?: number }).status ?? 0)) {
      stop('Koneksi perangkat dicabut atau sudah berakhir. Hubungkan kembali dengan kode baru.')
    } else {
      stopCamera()
      message.value = 'Koneksi terganggu. Perangkat akan mencoba terhubung kembali secara otomatis.'
    }
  }
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
    toast.success('Perangkat berhasil dipasangkan.')
  } catch (e) {
    toast.error(apiMessage(e))
  } finally {
    busy.value = false
  }
}
async function discover() {
  if (disposed || discovering || kiosk.value) return
  discovering = true
  sessions.value = []
  try {
    await auth.load()
    if (disposed || kiosk.value) return
    if (
      auth.session.value?.permissions.includes('devices.health') ||
      auth.session.value?.permissions.includes('devices.manage')
    ) {
      const data = await options<AttendanceSession>('/kiosk/session-options')
      if (!disposed) sessions.value = data
    } else message.value = 'Operator perlu masuk untuk memasangkan perangkat.'
  } catch {
    if (!disposed && !kiosk.value)
      message.value = 'Daftar sesi belum tersedia. Periksa koneksi dan akses operator.'
  } finally {
    discovering = false
  }
}
watch(kiosk, (value) => {
  if (!value && initialized && !disposed) void discover()
})
async function reconnect() {
  if (disposed) return
  if (kiosk.value) return refreshConnection()
  try {
    const result = await request<Result<Kiosk>>('/kiosk/me')
    if (!disposed && !kiosk.value && !busy.value) {
      arm(result.data)
      message.value = ''
    }
  } catch {
    // A missing device credential still needs the initial operator pairing.
  }
}
onMounted(async () => {
  document.addEventListener('visibilitychange', hidden)
  window.addEventListener('offline', offline)
  window.addEventListener('online', reconnect)
  await reconnect()
  if (disposed) return
  initialized = true
  if (!kiosk.value) await discover()
  clockTimer = setInterval(() => {
    clock.value = Date.now() + clockOffset
  }, 1000)
})
onBeforeUnmount(() => {
  disposed = true
  clearInterval(clockTimer)
  stop()
  document.removeEventListener('visibilitychange', hidden)
  window.removeEventListener('offline', offline)
  window.removeEventListener('online', reconnect)
})
</script>
<template>
  <main class="min-h-svh bg-muted/40 px-4 py-6 sm:px-8 sm:py-10">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <header class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p class="text-sm font-medium text-muted-foreground">Absensi sekolah</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-tight">
            {{
              kiosk
                ? sessionReady
                  ? kiosk.session_name
                  : 'Menunggu sesi absensi'
                : 'Pasangkan perangkat'
            }}
          </h1>
        </div>
        <div
          v-if="kiosk"
          class="ml-auto text-right"
        >
          <p
            class="flex items-center justify-end gap-2 text-3xl font-semibold tabular-nums tracking-tight"
          >
            <Clock class="size-5 text-muted-foreground" />
            <time>{{ formatTime(clock, true) }}</time>
          </p>
          <p class="mt-1 text-sm text-muted-foreground">{{ schoolDate }} · {{ kiosk.timezone }}</p>
        </div>
        <Badge
          v-else
          variant="secondary"
        >
          <ShieldCheck />
          Belum terhubung
        </Badge>
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
          <CardTitle>Hubungkan perangkat</CardTitle>
          <CardDescription>
            Masuk sebagai operator, pilih sesi terbuka dan masukkan kode penghubung yang diterbitkan
            admin. Pemasangan cukup sekali pada browser ini; sesi berikutnya untuk kelas yang sama
            terhubung otomatis.
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
            <Alert v-if="auth.session.value && !sessions.length">
              <AlertTitle>Belum ada sesi absensi terbuka</AlertTitle>
              <AlertDescription>
                Minta petugas membuka sesi pada jadwal yang sesuai sebelum memasangkan perangkat.
              </AlertDescription>
            </Alert>
            <FieldGroup>
              <Field :data-invalid="!!errors.pairing_token">
                <FieldLabel for="kiosk-token">Kode penghubung</FieldLabel>
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
      <Card
        v-else-if="!sessionReady"
        class="mx-auto w-full max-w-lg"
      >
        <CardHeader>
          <CardTitle>Perangkat sudah terhubung</CardTitle>
          <CardDescription>{{ kiosk.device_name }} · {{ kiosk.class_name }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-3">
          <Badge variant="secondary">
            <ShieldCheck />
            Koneksi tersimpan
          </Badge>
          <p
            role="status"
            class="text-sm text-muted-foreground"
          >
            {{
              kiosk.connection_status === 'multiple_sessions'
                ? 'Ada beberapa sesi aktif untuk kelas ini. Minta petugas menyisakan satu sesi yang sesuai.'
                : 'Belum ada sesi aktif untuk kelas ini. Halaman akan mengikuti sesi berikutnya secara otomatis.'
            }}
          </p>
          <p class="text-sm text-muted-foreground">
            Tidak perlu memasukkan kode lagi. Kamera dapat diaktifkan saat sesi siap.
          </p>
        </CardContent>
      </Card>
      <template v-else>
        <div class="flex flex-wrap items-center gap-2">
          <Badge :variant="departure ? 'info' : 'default'">
            <LogOut v-if="departure" />
            <LogIn v-else />
            {{ departure ? 'Absensi pulang' : 'Absensi masuk' }}
          </Badge>
          <Badge variant="outline">{{ kiosk.class_name }}</Badge>
          <p class="text-sm text-muted-foreground">
            {{
              kiosk.checkout_at
                ? `Otomatis beralih ke pulang pukul ${formatTime(kiosk.checkout_at)}`
                : 'Jam pulang belum diatur; perangkat mencatat masuk.'
            }}
            · Tutup {{ formatTime(kiosk.closes_at) }}
          </p>
        </div>
        <ol
          aria-label="Cara absensi"
          class="grid gap-2 text-sm text-muted-foreground sm:grid-cols-3"
        >
          <li
            v-for="(step, index) in [
              'Satu siswa menghadap kamera',
              'Tunggu nama dan konfirmasi berhasil',
              'Selesai, lanjut siswa berikutnya',
            ]"
            :key="step"
            class="flex items-center gap-2"
          >
            <Badge variant="secondary">{{ index + 1 }}</Badge>
            {{ step }}
          </li>
        </ol>
        <Alert v-if="!kiosk.recognition_enabled">
          <AlertTitle>Layanan wajah belum siap</AlertTitle>
          <AlertDescription>Untuk sementara, minta petugas mencatat kehadiran.</AlertDescription>
        </Alert>
        <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]">
          <Card class="min-w-0">
            <CardHeader
              class="has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]"
            >
              <CardTitle>Kamera absensi</CardTitle>
              <CardDescription>
                {{ kiosk.device_name }} · Hadapkan wajah ke kamera, lalu tunggu hasilnya.
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
              <div
                class="flex min-h-10 items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm"
                role="status"
                aria-live="polite"
              >
                <CircleCheck
                  v-if="cameraActive && faces.some((face) => face.accepted)"
                  class="size-4 shrink-0 text-success-foreground"
                />
                <span>
                  {{
                    cameraActive
                      ? scanMessage || 'Memeriksa wajah…'
                      : 'Kamera berhenti. Aktifkan kamera untuk mulai absensi.'
                  }}
                </span>
              </div>
            </CardContent>
            <CardFooter class="border-t text-xs text-muted-foreground">
              Perangkat terhubung sampai {{ formatTime(kiosk.expires_at) }}. Petugas dapat
              memasangkan ulang setelahnya.
            </CardFooter>
          </Card>
          <div class="flex min-w-0 flex-col gap-6">
            <Card
              role="region"
              aria-label="Konfirmasi absensi"
              :class="
                notice
                  ? notice.saved
                    ? 'border-success-foreground/40 bg-success'
                    : 'border-warning-foreground/40 bg-warning'
                  : ''
              "
            >
              <CardHeader>
                <CardTitle
                  class="flex items-center gap-2 text-xl"
                  :class="notice?.saved ? 'text-success-foreground' : ''"
                >
                  <CircleCheck
                    v-if="notice?.saved"
                    class="size-6 shrink-0"
                  />
                  {{ notice?.title ?? 'Hasil absensi terakhir' }}
                </CardTitle>
                <CardDescription>
                  {{ notice?.message ?? 'Jam tersimpan mengikuti waktu sekolah.' }}
                </CardDescription>
              </CardHeader>
              <CardContent
                v-if="lastResult && feedback"
                class="space-y-5"
              >
                <div class="flex items-center gap-4">
                  <StudentFacePhoto
                    :image="lastResult.referencePhoto"
                    :name="lastResult.name"
                    class="size-20 rounded-lg"
                  />
                  <div class="min-w-0 space-y-2">
                    <Badge :variant="feedback.tone">{{ feedback.status }}</Badge>
                    <p class="break-words text-3xl font-semibold tracking-tight">
                      {{ lastResult.name }}
                    </p>
                    <p class="text-sm text-muted-foreground">{{ feedback.message }}</p>
                  </div>
                </div>
                <div
                  v-if="lastResult.similarity !== undefined"
                  class="space-y-2"
                >
                  <dl class="flex items-center justify-between gap-3">
                    <dt class="text-sm text-muted-foreground">Skor kemiripan</dt>
                    <dd class="text-xl font-semibold tabular-nums">
                      {{
                        lastResult.similarity.toLocaleString('id-ID', {
                          minimumFractionDigits: 2,
                          maximumFractionDigits: 2,
                        })
                      }}
                    </dd>
                  </dl>
                  <p class="text-xs text-muted-foreground">
                    Tangkapan pertama yang terverifikasi · skala 0–1.
                  </p>
                </div>
                <Separator />
                <dl class="grid grid-cols-2 gap-4">
                  <div>
                    <dt class="text-sm text-muted-foreground">Jam masuk</dt>
                    <dd class="mt-1 text-xl font-semibold tabular-nums">
                      {{ formatTime(lastResult.attendance.record?.check_in_at, true) }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-sm text-muted-foreground">Jam pulang</dt>
                    <dd class="mt-1 text-xl font-semibold tabular-nums">
                      {{ formatTime(lastResult.attendance.record?.check_out_at, true) }}
                    </dd>
                  </div>
                </dl>
                <p
                  v-if="lastResult.attendance.record?.check_out_at"
                  class="text-xs text-muted-foreground"
                >
                  Status kehadiran tetap
                  {{ lastResult.attendance.record.status === 'late' ? 'terlambat' : 'hadir' }}; jam
                  pulang sudah tersimpan.
                </p>
              </CardContent>
              <CardContent v-else>
                <Empty class="border border-dashed">
                  <EmptyHeader>
                    <EmptyMedia variant="icon"><Camera /></EmptyMedia>
                    <EmptyTitle>Siap menerima siswa</EmptyTitle>
                    <EmptyDescription>
                      Nama, skor, dan waktu akan tampil setelah wajah cocok. Satu siswa bergantian
                      di depan kamera.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </CardContent>
            </Card>
            <Card v-if="recent.length">
              <CardHeader>
                <CardTitle>Baru tercatat</CardTitle>
                <CardDescription>Hasil pada perangkat ini sejak dipasangkan.</CardDescription>
              </CardHeader>
              <CardContent>
                <ul class="divide-y">
                  <li
                    v-for="item in recent"
                    :key="item.key"
                    class="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-sm font-medium">{{ item.name }}</p>
                      <p class="text-xs text-muted-foreground">
                        {{ item.departed ? 'Pulang' : 'Masuk' }}
                      </p>
                    </div>
                    <time class="shrink-0 text-sm tabular-nums">
                      {{ formatTime(item.time, true) }}
                    </time>
                  </li>
                </ul>
              </CardContent>
            </Card>
          </div>
        </div>
      </template>
    </div>
  </main>
</template>
