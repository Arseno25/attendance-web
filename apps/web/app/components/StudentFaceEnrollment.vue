<script setup lang="ts">
import { toast } from 'vue-sonner'
import { Ellipsis, RefreshCw, Trash2 } from '@lucide/vue'
import { enrollmentSchema, reasonSchema } from '~/lib/validation/biometrics-devices'
import type { Result } from '~/types/auth'
import type { FaceStatus } from '~/types/biometrics'

const props = defineProps<{ studentId: string; studentName?: string; open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()
const request = useApi()
const auth = useAuth()
const canEnroll = computed(() => auth.session.value?.permissions.includes('biometrics.enroll'))
const canRevoke = computed(() => auth.session.value?.permissions.includes('biometrics.revoke'))
const status = ref<FaceStatus | null>(null)
const revokeOpen = ref(false)
const busy = ref(false)
const loading = ref(false)
const error = ref('')
const frames = ref<string[]>([])
const reason = ref('')
const capturePanel = ref<{ stop: () => void } | null>(null)
const { errors, validate, clear } = useFormValidation(
  () => (revokeOpen.value ? reasonSchema : enrollmentSchema),
  () => (revokeOpen.value ? { reason: reason.value } : { frames: frames.value }),
)

let statusRequest: AbortController | undefined
const enrollmentReady = computed(
  () =>
    props.open &&
    !revokeOpen.value &&
    !!canEnroll.value &&
    !!status.value?.enrollment_allowed &&
    !!status.value?.pipeline_ready &&
    !loading.value,
)

async function refresh() {
  statusRequest?.abort()
  const controller = new AbortController()
  statusRequest = controller
  loading.value = true
  status.value = null
  error.value = ''
  try {
    const result = await request<Result<FaceStatus>>(`/students/${props.studentId}/face-status`, {
      signal: controller.signal,
    })
    if (!controller.signal.aborted) status.value = result.data
  } catch (cause) {
    if (!controller.signal.aborted) error.value = apiMessage(cause)
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}
function close(value: boolean) {
  if (!busy.value && !revokeOpen.value) emit('update:open', value)
}

async function save() {
  if (busy.value || !enrollmentReady.value || !validate()) return
  busy.value = true
  error.value = ''
  try {
    await request(`/students/${props.studentId}/enrollments`, {
      method: 'POST',
      body: { frames: frames.value },
    })
    capturePanel.value?.stop()
    frames.value = []
    busy.value = false
    emit('update:open', false)
    toast.success('Wajah berhasil didaftarkan. Gunakan halaman absensi pada sesi yang terbuka.')
  } catch (cause) {
    toast.error(apiMessage(cause))
  } finally {
    busy.value = false
  }
}

async function revoke() {
  if (busy.value || !canRevoke.value || !status.value?.enrolled || !validate()) return
  busy.value = true
  error.value = ''
  try {
    await request(`/students/${props.studentId}/face-revoke`, {
      method: 'POST',
      body: { reason: reason.value.trim() },
    })
    revokeOpen.value = false
    reason.value = ''
    toast.success('Data wajah dihapus. Riwayat kehadiran tetap tersedia.')
    await refresh()
  } catch (cause) {
    toast.error(apiMessage(cause))
  } finally {
    busy.value = false
  }
}

watch(
  () => [props.open, props.studentId],
  () => {
    statusRequest?.abort()
    capturePanel.value?.stop()
    frames.value = []
    revokeOpen.value = false
    clear()
    if (props.open) void refresh()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  statusRequest?.abort()
  capturePanel.value?.stop()
})
function openRevocation() {
  capturePanel.value?.stop()
  frames.value = []
  clear()
  reason.value = ''
  error.value = ''
  revokeOpen.value = true
}
</script>

<template>
  <Dialog
    :open="open && !revokeOpen"
    @update:open="close"
  >
    <DialogContent
      :show-close-button="!busy"
      class="flex max-h-[calc(100dvh-2rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl"
      @escape-key-down="
        (event) => {
          if (busy) event.preventDefault()
        }
      "
      @interact-outside="
        (event) => {
          if (busy) event.preventDefault()
        }
      "
    >
      <DialogHeader class="shrink-0 px-6 pt-6 pr-12 text-left">
        <DialogTitle>{{ canEnroll ? 'Daftar wajah' : 'Wajah siswa' }}</DialogTitle>
        <DialogDescription>
          {{ studentName || 'Siswa' }} · Ambil 3–5 foto dengan pencahayaan yang baik.
        </DialogDescription>
      </DialogHeader>
      <form
        id="face-enrollment"
        novalidate
        class="min-h-0 overflow-y-auto px-6 py-4 space-y-4"
        @submit.prevent="save"
      >
        <div class="flex items-center gap-3">
          <StudentFacePhoto
            v-if="status?.enrolled"
            :student-id="studentId"
            :revision="status.template?.reference_photo_version"
            :name="studentName || 'siswa'"
            class="size-14 shrink-0 rounded-lg"
          />
          <div class="min-w-0 flex-1 space-y-1">
            <Badge :variant="status?.enrolled ? 'success' : 'secondary'">
              {{
                loading
                  ? 'Memuat status…'
                  : status?.enrolled
                    ? 'Sudah terdaftar'
                    : 'Belum terdaftar'
              }}
            </Badge>
            <template v-if="status?.enrolled">
              <p class="text-sm font-medium">Foto referensi pendaftaran</p>
              <p class="text-xs text-muted-foreground">
                {{
                  status.template?.reference_photo_version
                    ? 'Ditampilkan saat wajah cocok pada absensi.'
                    : 'Foto belum tersedia. Perbarui pendaftaran untuk menambahkan foto.'
                }}
              </p>
            </template>
          </div>
          <DropdownMenu v-if="canRevoke && status?.enrolled">
            <DropdownMenuTrigger as-child>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                :disabled="busy || loading"
                aria-label="Opsi pendaftaran wajah"
              >
                <Ellipsis />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                variant="destructive"
                @select="openRevocation"
              >
                <Trash2 />
                Hapus data wajah
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <Alert
          v-if="error"
          variant="destructive"
          role="alert"
        >
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <Alert v-if="!loading && status && !enrollmentReady && canEnroll">
          <AlertTitle>Pendaftaran belum tersedia</AlertTitle>
          <AlertDescription>
            {{
              !status.enrollment_allowed
                ? 'Siswa harus aktif sebelum pendaftaran wajah.'
                : 'Kamera belum siap digunakan. Coba lagi sebentar.'
            }}
          </AlertDescription>
        </Alert>
        <Button
          v-if="!loading && (!status || (canEnroll && !status.pipeline_ready))"
          type="button"
          variant="outline"
          size="sm"
          :disabled="busy"
          @click="refresh"
        >
          <RefreshCw />
          Coba lagi
        </Button>
        <FaceEnrollmentCapture
          v-if="canEnroll"
          ref="capturePanel"
          v-model:frames="frames"
          :student-id="studentId"
          :enabled="enrollmentReady && open && !revokeOpen"
          :busy="busy"
        />
        <FieldError
          v-if="canEnroll && errors.frames"
          role="alert"
        >
          {{ errors.frames }}
        </FieldError>
      </form>
      <DialogFooter class="shrink-0 border-t px-6 py-4 sm:items-center sm:justify-between">
        <p
          v-if="canEnroll"
          class="text-sm text-muted-foreground"
          role="status"
        >
          {{
            frames.length < 3
              ? `Ambil ${3 - frames.length} foto lagi untuk menyimpan.`
              : 'Foto siap disimpan.'
          }}
        </p>
        <Button
          v-if="canEnroll"
          type="submit"
          form="face-enrollment"
          :disabled="busy || !enrollmentReady || frames.length < 3"
        >
          <Spinner v-if="busy" />
          Simpan wajah
        </Button>
        <Button
          v-else
          type="button"
          variant="outline"
          :disabled="busy"
          @click="close(false)"
        >
          Tutup
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
  <AlertDialog
    :open="revokeOpen"
    @update:open="
      (value) => {
        if (!busy) revokeOpen = value
      }
    "
  >
    <AlertDialogContent
      @escape-key-down="
        (event) => {
          if (busy) event.preventDefault()
        }
      "
    >
      <AlertDialogHeader>
        <AlertDialogTitle>Hapus data wajah?</AlertDialogTitle>
        <AlertDialogDescription>
          Pendaftaran ulang diperlukan untuk menggunakan wajah ini kembali. Riwayat kehadiran tetap
          tersedia.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <form
        novalidate
        class="space-y-4"
        @submit.prevent="revoke"
      >
        <Alert
          v-if="error"
          variant="destructive"
          role="alert"
        >
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <Field :data-invalid="!!errors.reason">
          <FieldLabel for="face-revoke-reason">Alasan</FieldLabel>
          <Textarea
            id="face-revoke-reason"
            v-model="reason"
            :disabled="busy"
            :aria-invalid="!!errors.reason"
            :aria-describedby="errors.reason ? 'face-revoke-error' : undefined"
          />
          <FieldError
            v-if="errors.reason"
            id="face-revoke-error"
          >
            {{ errors.reason }}
          </FieldError>
        </Field>
        <AlertDialogFooter>
          <AlertDialogCancel as-child>
            <Button
              type="button"
              variant="outline"
              :disabled="busy"
            >
              Batal
            </Button>
          </AlertDialogCancel>
          <Button
            type="submit"
            variant="destructive"
            :disabled="busy"
          >
            <Spinner v-if="busy" />
            Hapus data wajah
          </Button>
        </AlertDialogFooter>
      </form>
    </AlertDialogContent>
  </AlertDialog>
</template>
