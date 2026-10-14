<script setup lang="ts">
import { Camera, CameraOff, Check, RefreshCw } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { FaceBox, FaceOverlay } from '~/types/biometrics'
import { faceOverlays } from '~/lib/biometrics'

const props = defineProps<{
  studentId: string
  enabled: boolean
  busy: boolean
  frames: string[]
}>()
const emit = defineEmits<{ 'update:frames': [frames: string[]] }>()
const request = useApi()
const camera = ref<{ start: () => Promise<void>; stop: () => void; capture: () => string } | null>(
  null,
)
const active = ref(false)
const starting = ref(false)
const checking = ref(false)
const quality = ref<'ready' | 'no_face' | 'multiple_faces' | 'low_quality' | null>(null)
const message = ref('Aktifkan kamera untuk memeriksa posisi dan kualitas wajah.')
const error = ref('')
const faces = ref<FaceOverlay[]>([])
const checkedFrame = ref('')
let checkedAt = 0
let generation = 0
let timer: ReturnType<typeof setTimeout> | undefined
let abort: AbortController | undefined
let disposed = false
const canCapture = computed(
  () =>
    props.enabled &&
    !props.busy &&
    !checking.value &&
    !!checkedFrame.value &&
    props.frames.length < 5,
)

function stopPreview() {
  generation++
  clearTimeout(timer)
  abort?.abort()
  checkedFrame.value = ''
  faces.value = []
  quality.value = null
  checking.value = false
}
function stop() {
  stopPreview()
  camera.value?.stop()
}
function changed(value: boolean) {
  active.value = value
  stopPreview()
  if (!value) message.value = 'Kamera berhenti. Aktifkan kembali untuk mengambil foto.'
  if (value && props.enabled && !disposed) void check(generation)
}

async function check(version: number) {
  if (
    disposed ||
    version !== generation ||
    !active.value ||
    !props.enabled ||
    props.busy ||
    props.frames.length >= 5
  )
    return
  checking.value = true
  checkedFrame.value = ''
  faces.value = []
  error.value = ''
  abort = new AbortController()
  try {
    const image = camera.value?.capture()
    if (!image) return
    const result = await request<
      Result<{
        faces: { box: FaceBox }[]
        face_count: number
        quality: Exclude<typeof quality.value, null>
        message: string
      }>
    >(`/students/${props.studentId}/face-preview`, {
      method: 'POST',
      body: { image },
      signal: abort.signal,
    })
    if (disposed || version !== generation || props.busy) return
    quality.value = result.data.quality
    message.value = result.data.message
    faces.value = faceOverlays({
      decision: 'unknown',
      recognition_enabled: false,
      block_reason: null,
      faces: result.data.faces,
    }).map<FaceOverlay>((face) => ({
      ...face,
      label: quality.value === 'ready' ? 'Wajah siap' : 'Periksa posisi',
      tone: quality.value === 'ready' ? 'success' : 'warning',
    }))
    if (quality.value === 'ready' && result.data.face_count === 1 && faces.value.length === 1) {
      checkedFrame.value = image
      checkedAt = Date.now()
    }
  } catch (cause) {
    if (disposed || version !== generation) return
    quality.value = null
    error.value = apiMessage(cause)
    if ([401, 403, 409, 410].includes((cause as { status?: number }).status ?? 0)) {
      stop()
      return
    }
  } finally {
    if (!disposed && version === generation) {
      checking.value = false
      if (active.value && props.enabled && !props.busy && props.frames.length < 5)
        timer = setTimeout(() => void check(version), 1000)
    }
  }
}

function capture() {
  if (!canCapture.value || Date.now() - checkedAt > 1500) return
  emit('update:frames', [...props.frames, checkedFrame.value])
  checkedFrame.value = ''
}
async function reset() {
  stopPreview()
  const version = generation
  emit('update:frames', [])
  error.value = ''
  await nextTick()
  if (version === generation) void check(version)
}
watch(
  () => props.studentId,
  () => {
    stop()
    active.value = false
    emit('update:frames', [])
  },
)
watch(
  () => [props.enabled, props.busy],
  () => {
    stopPreview()
    if (!props.enabled) camera.value?.stop()
    else if (!props.busy && active.value) void check(generation)
  },
)
onBeforeUnmount(() => {
  disposed = true
  stop()
})
defineExpose({ stop })
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <Badge :variant="quality === 'ready' ? 'success' : 'secondary'">
        <Check v-if="quality === 'ready'" />
        {{
          checking ? 'Memeriksa foto…' : quality === 'ready' ? 'Kualitas siap' : 'Pemeriksaan wajah'
        }}
      </Badge>
      <Button
        type="button"
        variant="outline"
        :disabled="!enabled || busy || starting"
        @click="active ? stop() : camera?.start()"
      >
        <Spinner v-if="starting" />
        <CameraOff v-else-if="active" />
        <Camera v-else />
        {{ active ? 'Hentikan kamera' : 'Aktifkan kamera' }}
      </Button>
    </div>
    <FaceCamera
      ref="camera"
      :faces="faces"
      guide
      @active="changed"
      @busy="starting = $event"
    />
    <Alert
      v-if="error"
      variant="destructive"
      role="alert"
    >
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <p
      role="status"
      aria-live="polite"
      class="text-sm text-muted-foreground"
    >
      {{ message }}
    </p>
    <div class="space-y-2">
      <div class="flex items-center justify-between text-sm">
        <span>Foto pendaftaran</span>
        <span class="tabular-nums">{{ frames.length }} / 5 · minimal 3</span>
      </div>
      <Progress
        :model-value="frames.length * 20"
        aria-label="Progres foto pendaftaran"
      />
    </div>
    <div
      class="grid grid-cols-5 gap-2"
      aria-label="Foto sementara"
    >
      <div
        v-for="index in 5"
        :key="index"
        class="relative flex aspect-video items-center justify-center overflow-hidden rounded-md border bg-muted text-sm text-muted-foreground"
      >
        <img
          v-if="frames[index - 1]"
          :src="`data:image/jpeg;base64,${frames[index - 1]}`"
          :alt="`Foto ${index}`"
          class="h-full w-full object-cover"
        />
        <span v-else>{{ index }}</span>
      </div>
    </div>
    <p class="text-sm text-muted-foreground">
      Hadap depan, lalu ubah posisi kepala sedikit untuk foto berikutnya. Pastikan hanya satu wajah
      terlihat jelas. Semua foto harus menampilkan siswa yang sama.
    </p>
    <div class="flex flex-wrap justify-end gap-2">
      <Button
        type="button"
        variant="outline"
        :disabled="busy || !frames.length"
        @click="reset"
      >
        <RefreshCw />
        Ulangi foto
      </Button>
      <Button
        type="button"
        :disabled="!canCapture"
        @click="capture"
      >
        <Camera />
        Ambil foto
      </Button>
    </div>
  </div>
</template>
