<script setup lang="ts">
import { deviceSchema, reasonSchema } from '~/lib/validation/consent-devices'
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
const success = ref('')
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
  success.value = ''
  try {
    if (kind === 'pair')
      pairing.value = (
        await request<Result<{ pairing_token: string; expires_at: string }>>(
          `/devices/${id}/pairing-token`,
          { method: 'POST' },
        )
      ).data
    else {
      await request(kind === 'create' ? '/devices' : `/devices/${revoking.value?.id}/revoke`, {
        method: 'POST',
        body: kind === 'create' ? form : { reason: reason.value },
      })
      editor.value = false
      revoking.value = null
      success.value =
        kind === 'create'
          ? 'Perangkat didaftarkan.'
          : 'Perangkat dicabut; sesi kiosk langsung tidak berlaku.'
      await load()
    }
  } catch (e) {
    error.value = apiMessage(e)
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
      title="Perangkat kiosk"
      description="Daftarkan perangkat, pasangkan pada sesi dan pantau heartbeat."
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
    <Alert
      v-if="success"
      role="status"
    >
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Perangkat terdaftar</CardTitle>
        <CardDescription>
          Token pairing hanya berlaku sekali selama 5 menit. Sesi kiosk berlaku 10 menit dan harus
          dipasangkan ulang oleh operator.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table class="min-w-[600px]">
          <TableCaption class="sr-only">Perangkat dan heartbeat terakhir</TableCaption>
          <TableHeader class="bg-muted/50">
            <TableRow>
              <TableHead>Perangkat</TableHead>
              <TableHead>Lokasi</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Heartbeat terakhir</TableHead>
              <TableHead class="w-12 text-right">Tindakan</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="item in devices"
              :key="item.id"
            >
              <TableCell class="font-medium">{{ item.name }}</TableCell>
              <TableCell>{{ item.location }}</TableCell>
              <TableCell>
                <Badge :variant="statusVariant(item.revoked_at ? 'revoked' : 'active')">
                  {{ item.revoked_at ? 'Dicabut' : 'Terdaftar' }}
                </Badge>
              </TableCell>
              <TableCell>
                {{ item.last_seen_at ? dateTime(item.last_seen_at) : 'Belum terhubung' }}
              </TableCell>
              <TableCell class="text-right">
                <TableActions
                  v-if="canManage && !item.revoked_at"
                  :label="item.name"
                  :disabled="busy"
                >
                  <DropdownMenuItem @select="act('pair', item.id)">Token pairing</DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    @select="openRevocation(item)"
                  >
                    Cabut perangkat
                  </DropdownMenuItem>
                </TableActions>
                <span
                  v-else
                  class="text-muted-foreground"
                >
                  —
                </span>
              </TableCell>
            </TableRow>
            <TableRow v-if="!devices.length">
              <TableCell
                colspan="5"
                class="py-8 text-center text-muted-foreground"
              >
                {{ loading ? 'Memuat perangkat…' : 'Belum ada perangkat.' }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
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
            Perangkat baru memerlukan pairing sebelum digunakan.
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
            Perangkat {{ revoking?.name }} dan semua sesi pairing akan langsung dinonaktifkan.
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
          <DialogTitle>Token pairing sekali pakai</DialogTitle>
          <DialogDescription>
            Masukkan token ini di halaman absensi pada perangkat tujuan. Token tidak disimpan di
            browser.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel for="pairing-token">Token pairing</FieldLabel>
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
