<script setup lang="ts">
import { toast } from 'vue-sonner'
import { RefreshCw, ScanFace, Trash2 } from '@lucide/vue'
import { enrollmentSchema, reasonSchema } from '~/lib/validation/consent-devices'
import type { Result } from '~/types/auth'
import type { FaceStatus } from '~/types/biometrics'

const props = defineProps<{ studentId: string; studentName?: string }>()
const request = useApi()
const auth = useAuth()
const canEnroll = computed(() => auth.session.value?.permissions.includes('biometrics.enroll'))
const canRevoke = computed(() => auth.session.value?.permissions.includes('biometrics.revoke'))
const status = ref<FaceStatus | null>(null)
const open = ref(false)
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

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    status.value = (
      await request<Result<FaceStatus>>(`/students/${props.studentId}/face-status`)
    ).data
  } catch (cause) {
    status.value = null
    error.value = apiMessage(cause)
  } finally {
    loading.value = false
  }
}

async function register() {
  await refresh()
  frames.value = []
  clear()

  open.value = true
}

async function save() {
  if (busy.value || !validate()) return
  busy.value = true
  error.value = ''
  try {
    await request(`/students/${props.studentId}/enrollments`, {
      method: 'POST',
      body: { frames: frames.value },
    })
    capturePanel.value?.stop()
    frames.value = []
    open.value = false
    toast.success(
      'Wajah terdaftar sebagai template provisional. Absensi otomatis menunggu evaluasi.',
    )
    await refresh()
  } catch (cause) {
    toast.error(apiMessage(cause))
  } finally {
    busy.value = false
  }
}

async function revoke() {
  if (busy.value || !validate()) return
  busy.value = true
  error.value = ''
  try {
    await request(`/students/${props.studentId}/face-revoke`, {
      method: 'POST',
      body: { reason: reason.value.trim() },
    })
    revokeOpen.value = false
    reason.value = ''
    toast.success('Template wajah dihapus. Riwayat kehadiran tetap tersedia.')
    await refresh()
  } catch (cause) {
    toast.error(apiMessage(cause))
  } finally {
    busy.value = false
  }
}

watch(open, (value) => {
  if (!value) {
    capturePanel.value?.stop()
    frames.value = []
    clear()
  }
})
onMounted(refresh)
defineExpose({ refresh })
function openRevocation() {
  clear()
  reason.value = ''
  error.value = ''
  revokeOpen.value = true
}
</script>

<template>
  <Card class="min-w-0">
    <CardHeader
      class="has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]"
    >
      <CardTitle>Wajah siswa</CardTitle>
      <CardDescription>
        Daftar setelah persetujuan wali terverifikasi. Foto hanya diproses saat pendaftaran;
        template disimpan terenkripsi.
      </CardDescription>
      <CardAction
        class="col-start-1 row-start-3 row-span-1 justify-self-end sm:col-start-2 sm:row-start-1 sm:row-span-2"
      >
        <div class="flex flex-wrap justify-end gap-2">
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="Muat ulang status wajah"
            :disabled="busy || loading"
            @click="refresh"
          >
            <RefreshCw :class="loading ? 'animate-spin' : ''" />
          </Button>
          <Button
            v-if="canRevoke && status?.enrolled"
            variant="outline"
            size="sm"
            :disabled="busy || loading"
            @click="openRevocation"
          >
            <Trash2 />
            Hapus template
          </Button>
          <Button
            v-if="canEnroll"
            size="sm"
            :disabled="busy || loading"
            @click="register"
          >
            <ScanFace />
            {{ status?.enrolled ? 'Daftar ulang' : 'Daftar wajah' }}
          </Button>
        </div>
      </CardAction>
    </CardHeader>
    <CardContent class="space-y-4">
      <Alert
        v-if="error && !open && !revokeOpen"
        variant="destructive"
        role="alert"
      >
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <dl class="grid gap-4 sm:grid-cols-3">
        <div>
          <dt class="text-sm text-muted-foreground">Pendaftaran</dt>
          <dd class="mt-2">
            <Badge :variant="status?.enrolled ? 'warning' : 'secondary'">
              {{
                !status
                  ? 'Status belum tersedia'
                  : status.enrolled
                    ? 'Terdaftar · provisional'
                    : 'Belum terdaftar'
              }}
            </Badge>
          </dd>
        </div>
        <div>
          <dt class="text-sm text-muted-foreground">Persetujuan</dt>
          <dd class="mt-2">
            <Badge :variant="status?.consent_eligible ? 'success' : 'secondary'">
              {{
                !status
                  ? 'Status belum tersedia'
                  : status.consent_eligible
                    ? 'Berlaku'
                    : 'Belum eligible'
              }}
            </Badge>
          </dd>
        </div>
        <div>
          <dt class="text-sm text-muted-foreground">Akurasi & liveness</dt>
          <dd class="mt-2"><Badge variant="warning">Belum dievaluasi</Badge></dd>
        </div>
      </dl>
      <p
        v-if="status?.template"
        class="text-sm text-muted-foreground"
      >
        {{ status.template.frame_count }} frame pendaftaran ·
        {{ new Date(status.template.created_at).toLocaleString('id-ID') }}
      </p>
      <Alert>
        <AlertTitle>Absensi otomatis menunggu evaluasi</AlertTitle>
        <AlertDescription>
          Uji kamera dengan sampel terpisah, serangan foto/video, dan kalibrasi ambang diperlukan
          sebelum pengenalan dapat mencatat kehadiran. Absensi manual tetap tersedia.
        </AlertDescription>
      </Alert>
      <p
        v-if="status && !status.pipeline_ready"
        class="text-sm text-muted-foreground"
      >
        Layanan pendaftaran wajah belum siap. Muat ulang setelah layanan pulih.
      </p>
    </CardContent>
  </Card>
  <Dialog
    :open="open"
    @update:open="
      (value) => {
        if (!busy) open = value
      }
    "
  >
    <DialogContent
      :show-close-button="!busy"
      class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
    >
      <DialogHeader>
        <DialogTitle>Daftar wajah{{ studentName ? ` · ${studentName}` : ' siswa' }}</DialogTitle>
        <DialogDescription>
          Ambil 3–5 frame berbeda dengan satu wajah, pencahayaan baik, dan posisi kepala sedikit
          bervariasi.
        </DialogDescription>
      </DialogHeader>
      <form
        novalidate
        class="space-y-4"
        @submit.prevent="save"
      >
        <Alert
          v-if="error"
          variant="destructive"
          role="alert"
        >
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <Alert v-if="!status?.consent_eligible || !status?.pipeline_ready">
          <AlertTitle>Pendaftaran belum tersedia</AlertTitle>
          <AlertDescription>
            {{
              !status?.consent_eligible
                ? 'Persetujuan wali harus berlaku dan siswa harus aktif sebelum kamera dapat digunakan untuk pendaftaran.'
                : 'Layanan model belum siap. Muat ulang status setelah layanan pulih.'
            }}
          </AlertDescription>
        </Alert>
        <FaceEnrollmentCapture
          ref="capturePanel"
          v-model:frames="frames"
          :student-id="studentId"
          :enabled="!!status?.consent_eligible && !!status?.pipeline_ready"
          :busy="busy"
        />
        <Field :data-invalid="!!errors.frames">
          <FieldDescription>
            Frame tidak disimpan sebagai foto. Pemeriksaan kualitas dilakukan sebelum template
            tersimpan.
          </FieldDescription>
          <FieldError
            v-if="errors.frames"
            role="alert"
          >
            {{ errors.frames }}
          </FieldError>
        </Field>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            :disabled="busy"
            @click="open = false"
          >
            Batal
          </Button>
          <Button
            type="submit"
            :disabled="busy || !status?.consent_eligible || !status?.pipeline_ready"
          >
            <Spinner v-if="busy" />
            Simpan wajah
          </Button>
        </DialogFooter>
      </form>
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
        <AlertDialogTitle>Hapus template wajah?</AlertDialogTitle>
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
            Hapus template
          </Button>
        </AlertDialogFooter>
      </form>
    </AlertDialogContent>
  </AlertDialog>
</template>
