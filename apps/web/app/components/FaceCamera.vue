<script setup lang="ts">
import { CameraOff } from '@lucide/vue'
import { captureJpeg } from '../lib/biometrics.ts'
import type { FaceOverlay } from '../types/biometrics.ts'

withDefaults(defineProps<{ faces?: FaceOverlay[]; guide?: boolean }>(), {
  faces: () => [],
  guide: false,
})
const emit = defineEmits<{ active: [value: boolean]; busy: [value: boolean] }>()
const video = ref<HTMLVideoElement | null>(null)
const active = ref(false)
const busy = ref(false)
const error = ref('')
const ratio = ref(16 / 9)
const cameraSelectId = useId()
const cameras = ref<MediaDeviceInfo[]>([])
const selectedCamera = ref('')
const cameraStorageKey = 'attendance-camera-device'
async function refreshCameras() {
  if (!navigator.mediaDevices?.enumerateDevices) return
  const devices = await navigator.mediaDevices.enumerateDevices()
  if (disposed) return
  cameras.value = devices.filter((device) => device.kind === 'videoinput' && device.deviceId)
  if (
    cameras.value.some((device) => device.label) &&
    !cameras.value.some((device) => device.deviceId === selectedCamera.value)
  )
    selectedCamera.value =
      cameras.value.find((device) => /fantech|fantect/i.test(device.label))?.deviceId ?? ''
}
let media: MediaStream | null = null
let generation = 0
let disposed = false

function stop() {
  generation++
  media?.getTracks().forEach((track) => track.stop())
  media = null
  if (video.value) video.value.srcObject = null
  active.value = false
  emit('active', false)
}

async function start() {
  if (busy.value || disposed || document.hidden || !navigator.onLine) return
  const request = ++generation
  busy.value = true
  emit('busy', true)
  error.value = ''
  try {
    const constraints = () => ({
      video: {
        ...(selectedCamera.value
          ? { deviceId: { exact: selectedCamera.value } }
          : { facingMode: 'user' }),
        width: { ideal: 1280 },
        height: { ideal: 720 },
      },
      audio: false,
    })
    let stream = await navigator.mediaDevices.getUserMedia(constraints())
    if (request !== generation || disposed || document.hidden || !navigator.onLine) {
      stream.getTracks().forEach((track) => track.stop())
      return
    }
    media = stream
    await refreshCameras()
    if (request !== generation || disposed) return
    const currentId = stream.getVideoTracks()[0]?.getSettings?.().deviceId
    if (selectedCamera.value && currentId && selectedCamera.value !== currentId) {
      stream.getTracks().forEach((track) => track.stop())
      media = null
      stream = await navigator.mediaDevices.getUserMedia(constraints())
      if (request !== generation || disposed || document.hidden || !navigator.onLine) {
        stream.getTracks().forEach((track) => track.stop())
        return
      }
      media = stream
    }
    selectedCamera.value ||= stream.getVideoTracks()[0]?.getSettings?.().deviceId ?? ''
    try {
      window.localStorage.setItem(cameraStorageKey, selectedCamera.value)
    } catch {
      /* Camera works when browser storage is disabled. */
    }
    stream.getVideoTracks().forEach((track) =>
      track.addEventListener('ended', () => {
        if (media === stream) stop()
      }),
    )
    if (!video.value) {
      stop()
      return
    }
    video.value.srcObject = stream
    await video.value.play()
    if (request !== generation || disposed) return
    resize()
    active.value = true
    emit('active', true)
  } catch {
    if (request === generation) {
      stop()
      error.value = 'Kamera tidak dapat digunakan. Periksa izin browser dan sambungan kamera.'
    }
  } finally {
    busy.value = false
    emit('busy', false)
  }
}

function resize() {
  if (video.value?.videoWidth && video.value.videoHeight)
    ratio.value = video.value.videoWidth / video.value.videoHeight
}
function capture() {
  if (!active.value || !video.value) throw new Error('Aktifkan kamera terlebih dahulu.')
  return captureJpeg(video.value)
}
function hidden() {
  if (document.hidden) stop()
}
onMounted(async () => {
  document.addEventListener('visibilitychange', hidden)
  window.addEventListener('offline', stop)
  try {
    selectedCamera.value = window.localStorage.getItem(cameraStorageKey) ?? ''
    await refreshCameras()
  } catch {
    /* Device labels may be unavailable until camera permission is granted. */
  }
})
onBeforeUnmount(() => {
  disposed = true
  stop()
  document.removeEventListener('visibilitychange', hidden)
  window.removeEventListener('offline', stop)
})
defineExpose({ start, stop, capture })
</script>

<template>
  <div class="space-y-3">
    <Field
      v-if="cameras.length"
      class="sm:max-w-sm"
    >
      <FieldLabel :for="cameraSelectId">Kamera</FieldLabel>
      <Select
        v-model="selectedCamera"
        :disabled="busy || active"
      >
        <SelectTrigger
          :id="cameraSelectId"
          class="w-full"
        >
          <SelectValue placeholder="Pilih kamera" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="(device, index) in cameras"
            :key="device.deviceId"
            :value="device.deviceId"
          >
            {{ device.label || `Kamera ${index + 1}` }}
          </SelectItem>
        </SelectContent>
      </Select>
      <FieldDescription v-if="active">Hentikan kamera untuk mengganti perangkat.</FieldDescription>
    </Field>
    <div
      class="relative overflow-hidden rounded-lg bg-muted"
      :style="{ aspectRatio: ratio }"
    >
      <video
        v-show="active"
        ref="video"
        class="h-full w-full object-contain"
        playsinline
        muted
        aria-label="Preview kamera"
        @loadedmetadata="resize"
        @resize="resize"
      />
      <div
        v-if="!active"
        class="absolute inset-0 flex flex-col items-center justify-center gap-3 text-muted-foreground"
      >
        <CameraOff class="size-10" />
        <p>Kamera belum aktif</p>
      </div>
      <template v-if="active">
        <div
          v-if="guide"
          aria-hidden="true"
          class="pointer-events-none absolute left-1/2 top-1/2 h-3/4 w-2/5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-dashed border-white/70"
        />
        <div
          v-for="(face, index) in faces"
          :key="index"
          class="absolute rounded border-2 transition-[left,top,width,height] duration-150 motion-reduce:transition-none"
          :class="
            face.tone === 'success' || face.accepted
              ? 'border-success-foreground'
              : 'border-warning-foreground'
          "
          :style="{
            left: `${face.box.x * 100}%`,
            top: `${face.box.y * 100}%`,
            width: `${face.box.width * 100}%`,
            height: `${face.box.height * 100}%`,
          }"
        >
          <Badge
            :variant="face.tone ?? (face.accepted ? 'success' : 'warning')"
            class="absolute left-0 top-0 max-w-full rounded-sm"
          >
            <span class="min-w-0 truncate">{{ face.label }}</span>
          </Badge>
        </div>
      </template>
    </div>
    <Alert
      v-if="error"
      variant="destructive"
      role="alert"
    >
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
  </div>
</template>
