<script setup lang="ts">
import { toast } from 'vue-sonner'
import { deviceSchema, reasonSchema } from '~/lib/validation/biometrics-devices'
import { statusVariant } from '~/lib/status'
import type { Result } from '~/types/auth'
definePageMeta({ layout: 'admin', middleware: ['auth', 'devices'] })
interface Device {
  id: string
  name: string
  location: string
  revoked_at: string | null
  last_seen_at: string | null
}
const { request, dateTime } = useMasterData()
const auth = useAuth()
const canManage = computed(() => auth.session.value?.permissions.includes('devices.manage'))
const devices = ref<Device[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const editor = ref(false)
const revoking = ref<Device | null>(null)
const reason = ref('')
const pairing = ref<{ pairing_token: string; expires_at: string } | null>(null)
const form = reactive({ name: '', location: '' })
const { errors, validate, clear } = useFormValidation(
  () => (editor.value ? deviceSchema : reasonSchema),
  () => (editor.value ? form : { reason: reason.value }),
)
async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await request<Result<Device[]>>('/devices', {
      query: { page: page.value, page_size: 20 },
    })
    devices.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (e) {
    devices.value = []
    error.value = apiMessage(e)
  } finally {
    loading.value = false
  }
}
async function act(kind: 'create' | 'revoke' | 'pair', id?: string) {
  if (busy.value || (kind !== 'pair' && !validate())) return
  busy.value = true
  error.value = ''

  try {
    if (kind === 'pair') {
      pairing.value = (
        await request<Result<{ pairing_token: string; expires_at: string }>>(
          `/devices/${id}/pairing-token`,
          { method: 'POST' },
        )
      ).data
      toast.success('Kode penghubung siap digunakan.')
    } else {
      await request(kind === 'create' ? '/devices' : `/devices/${revoking.value?.id}/revoke`, {
        method: 'POST',
        body: kind === 'create' ? form : { reason: reason.value },
      })
      editor.value = false
      revoking.value = null
      toast.success(
        kind === 'create'
          ? 'Perangkat didaftarkan.'
          : 'Perangkat dicabut; sesi absensi langsung tidak berlaku.',
      )
      await load()
    }
  } catch (e) {
    toast.error(apiMessage(e))
  } finally {
    busy.value = false
  }
}
await load()
function openDevice() {
  clear()
  Object.assign(form, { name: '', location: '' })
  error.value = ''
  editor.value = true
}
function openRevocation(item: Device) {
  clear()
  revoking.value = item
  reason.value = ''
  error.value = ''
}
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Perangkat absensi"
      description="Hubungkan perangkat untuk absensi dan lihat status koneksinya."
    >
      <template #actions>
        <div class="flex gap-2">
          <Button
            as-child
            variant="outline"
          >
            <NuxtLink
              to="/absensi"
              target="_blank"
            >
              Buka halaman absensi
            </NuxtLink>
          </Button>
          <Button
            v-if="canManage"
            @click="openDevice"
          >
            Tambah perangkat
          </Button>
        </div>
      </template>
    </PageHeader>
    <Alert
      v-if="error && !editor && !revoking"
      variant="destructive"
    >
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Perangkat terdaftar</CardTitle>
        <CardDescription>
          Kode penghubung berlaku sekali selama 5 menit. Perangkat perlu dihubungkan kembali setelah
          10 menit.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <DataTable
          class="min-w-[600px]"
          :data="devices ?? []"
          :columns="[
            { id: 'perangkat', label: 'Perangkat', cellClass: 'font-medium' },
            { id: 'lokasi', label: 'Lokasi' },
            { id: 'status', label: 'Status' },
            { id: 'heartbeat-terakhir', label: 'Koneksi terakhir' },
            {
              id: 'tindakan',
              label: 'Tindakan',
              headClass: 'w-24 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Perangkat dan koneksi terakhir"
          row-key="id"
          :loading="loading"
        >
          <template #cell-perangkat="{ item }">{{ item.name }}</template>
          <template #cell-lokasi="{ item }">{{ item.location }}</template>
          <template #cell-status="{ item }">
            <Badge :variant="statusVariant(item.revoked_at ? 'revoked' : 'active')">
              {{ item.revoked_at ? 'Dicabut' : 'Terdaftar' }}
            </Badge>
          </template>
          <template #cell-heartbeat-terakhir="{ item }">
            {{ item.last_seen_at ? dateTime(item.last_seen_at) : 'Belum terhubung' }}
          </template>
          <template #cell-tindakan="{ item }">
            <TableActions
              v-if="canManage && !item.revoked_at"
              :label="item.name"
              :disabled="busy"
              :actions="[
                {
                  label: 'Kode penghubung',
                  icon: 'pair',
                  onSelect: () => {
                    act('pair', item.id)
                  },
                },
                {
                  label: 'Cabut perangkat',
                  icon: 'revoke',
                  onSelect: () => {
                    openRevocation(item)
                  },
                  destructive: true,
                },
              ]"
            />
            <span
              v-else
              class="text-muted-foreground"
            >
              —
            </span>
          </template>
          <template #empty>
            {{ loading ? 'Memuat perangkat…' : 'Belum ada perangkat.' }}
          </template>
        </DataTable>
      </CardContent>
      <CardFooter class="block border-t">
        <TablePagination
          :page="page"
          :total="total"
          label="perangkat"
          :disabled="loading || busy"
          @update:page="
            (value) => {
              page = value
              load()
            }
          "
        />
      </CardFooter>
    </Card>
    <Dialog
      :open="editor"
      @update:open="
        (v) => {
          if (!busy) editor = v
        }
      "
    >
      <DialogContent
        :show-close-button="!busy"
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>Daftar perangkat</DialogTitle>
          <DialogDescription>
            Hubungkan perangkat dengan sesi absensi sebelum digunakan.
          </DialogDescription>
        </DialogHeader>
        <form
          novalidate
          class="flex flex-col gap-6"
          @submit.prevent="act('create')"
        >
          <Alert
            v-if="error"
            variant="destructive"
          >
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
          <FieldGroup>
            <Field :data-invalid="!!errors.name">
              <FieldLabel for="device-name">Nama perangkat</FieldLabel>
              <Input
                id="device-name"
                v-model="form.name"
                :aria-invalid="!!errors.name"
                :aria-describedby="errors.name ? 'device-name-error' : undefined"
                required
                maxlength="120"
                :disabled="busy"
              />
              <FieldError
                v-if="errors.name"
                id="device-name-error"
              >
                {{ errors.name }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.location">
              <FieldLabel for="device-location">Lokasi</FieldLabel>
              <Input
                id="device-location"
                v-model="form.location"
                :aria-invalid="!!errors.location"
                :aria-describedby="errors.location ? 'device-location-error' : undefined"
                required
                maxlength="120"
                :disabled="busy"
              />
              <FieldError
                v-if="errors.location"
                id="device-location-error"
              >
                {{ errors.location }}
              </FieldError>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="busy"
              @click="editor = false"
            >
              Batal
            </Button>
            <Button
              type="submit"
              :disabled="busy"
            >
              <Spinner v-if="busy" />
              Daftarkan
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <AlertDialog
      :open="!!revoking"
      @update:open="
        (v) => {
          if (!v && !busy) revoking = null
        }
      "
    >
      <AlertDialogContent
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        @escape-key-down="
          (event) => {
            if (busy) event.preventDefault()
          }
        "
      >
        <AlertDialogHeader>
          <AlertDialogTitle>Cabut perangkat</AlertDialogTitle>
          <AlertDialogDescription>
            Perangkat {{ revoking?.name }} tidak dapat digunakan untuk absensi setelah
            dinonaktifkan.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <form
          novalidate
          class="space-y-4"
          @submit.prevent="act('revoke')"
        >
          <Alert
            v-if="error"
            variant="destructive"
          >
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
          <Field :data-invalid="!!errors.reason">
            <FieldLabel for="device-reason">Alasan pencabutan</FieldLabel>
            <Textarea
              id="device-reason"
              v-model="reason"
              :aria-invalid="!!errors.reason"
              :aria-describedby="errors.reason ? 'device-reason-error' : undefined"
              required
              maxlength="500"
              :disabled="busy"
            />
            <FieldError
              v-if="errors.reason"
              id="device-reason-error"
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
                @click="revoking = null"
              >
                Batal
              </Button>
            </AlertDialogCancel>
            <Button
              type="submit"
              variant="destructive"
              :disabled="busy"
            >
              Cabut perangkat
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
    <Dialog
      :open="!!pairing"
      @update:open="
        (v) => {
          if (!v) pairing = null
        }
      "
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Kode penghubung sekali pakai</DialogTitle>
          <DialogDescription>
            Masukkan kode ini di halaman absensi pada perangkat yang ingin dihubungkan. Kode hanya
            dapat digunakan sekali.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel for="pairing-token">Kode penghubung</FieldLabel>
          <Input
            id="pairing-token"
            :model-value="pairing?.pairing_token"
            readonly
            autocomplete="off"
            class="font-mono text-xs"
          />
        </Field>
        <p class="text-sm text-muted-foreground">
          Berlaku sampai {{ pairing ? dateTime(pairing.expires_at) : '' }}.
        </p>
        <DialogFooter><Button @click="pairing = null">Selesai</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
