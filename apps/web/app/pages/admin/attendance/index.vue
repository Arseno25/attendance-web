<script setup lang="ts">
import { toast } from 'vue-sonner'
import { sessionSchema, sessionFilterSchema } from '~/lib/validation/attendance'
import { statusVariant } from '~/lib/status'
import { Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { sessionLabels, type AttendanceSession, type Schedule } from '~/types/attendance'
definePageMeta({ layout: 'admin', middleware: ['auth', 'attendance'] })
const { request, options, dateTime, canManual, today } = useAttendance()
const sessions = ref<AttendanceSession[]>([])
const schedules = ref<Schedule[]>([])
const page = ref(1)
const total = ref(0)
const date = ref(today())
const loading = ref(false)
const saving = ref(false)
const open = ref(false)
const message = ref('')
const form = reactive({ schedule_id: '', date: today() })
const { errors, validate, clear } = useFormValidation(
  () => sessionSchema,
  () => form,
)
const { errors: filterErrors, validate: validateFilter } = useFormValidation(
  () => sessionFilterSchema,
  () => ({ date: date.value }),
)
async function load() {
  if (!validateFilter()) return
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<AttendanceSession[]>>('/attendance-sessions', {
      query: { date: date.value, page: page.value, page_size: 20 },
    })
    sessions.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    loading.value = false
  }
}
async function show() {
  clear()
  message.value = ''
  form.schedule_id = ''
  form.date = date.value || today()
  try {
    schedules.value = (await options<Schedule>('/attendance-schedules')).filter(
      (item) => item.is_active,
    )
    open.value = true
  } catch (error) {
    message.value = apiMessage(error)
  }
}
async function save() {
  if (saving.value || !validate()) return
  saving.value = true
  message.value = ''
  try {
    const data = (
      await request<Result<AttendanceSession>>('/attendance-sessions', {
        method: 'POST',
        body: form,
      })
    ).data
    open.value = false
    toast.success('Sesi absensi berhasil dibuat.')
    await navigateTo(`/admin/attendance/${data.id}`)
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function filter() {
  page.value = 1
  await load()
}
async function changePage(value: number) {
  page.value = value
  await load()
}
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Sesi absensi"
      description="Catat kehadiran, tinjau yang belum tercatat, lalu rekonsiliasi setelah sesi ditutup."
    >
      <template #actions>
        <Button
          v-if="canManual"
          @click="show"
        >
          <Plus />
          Buat sesi
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="message && !open"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader>
        <div>
          <CardTitle>Sesi sekolah</CardTitle>
          <CardDescription class="mt-1.5">
            Sesi hari ini dibuat otomatis dari jadwal aktif. Semua tanggal mengikuti zona waktu
            sekolah.
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <DataTableToolbar @submit.prevent="filter">
          <Label
            for="session-date"
            class="sr-only"
          >
            Tanggal sesi
          </Label>
          <Field
            class="w-full sm:w-48"
            :data-invalid="!!filterErrors.date"
          >
            <Input
              id="session-date"
              v-model="date"
              :aria-invalid="!!filterErrors.date"
              :aria-describedby="filterErrors.date ? 'session-date-error' : undefined"
              type="date"
              :disabled="loading"
            />
            <FieldError
              v-if="filterErrors.date"
              id="session-date-error"
            >
              {{ filterErrors.date }}
            </FieldError>
          </Field>
          <Button
            variant="outline"
            type="submit"
            :disabled="loading"
          >
            Lihat
          </Button>
        </DataTableToolbar>
        <div
          v-if="loading"
          class="space-y-3"
          role="status"
        >
          <span class="sr-only">Memuat sesi</span>
          <Skeleton
            v-for="n in 4"
            :key="n"
            class="h-10"
          />
        </div>
        <DataTable
          v-else
          class="min-w-[720px]"
          :data="sessions ?? []"
          :columns="[
            { id: 'kelas-jadwal', label: 'Kelas / jadwal', cellClass: 'font-medium' },
            { id: 'periode', label: 'Periode' },
            { id: 'status', label: 'Status' },
            { id: 'tercatat-roster', label: 'Tercatat / siswa', cellClass: 'tabular-nums' },
            {
              id: 'tindakan',
              label: 'Tindakan',
              headClass: 'w-24 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Daftar sesi absensi sekolah"
          row-key="id"
          :loading="loading"
        >
          <template #cell-kelas-jadwal="{ item }">
            {{ item.class_name }}
            <div class="text-xs font-normal text-muted-foreground">
              {{ item.name }}
            </div>
          </template>
          <template #cell-periode="{ item }">
            {{ dateTime(item.starts_at) }}
            <div class="text-xs text-muted-foreground">Sampai {{ dateTime(item.closes_at) }}</div>
          </template>
          <template #cell-status="{ item }">
            <Badge :variant="statusVariant(item.status)">
              {{ sessionLabels[item.status] }}
            </Badge>
            <div
              v-if="item.reconciled_at"
              class="mt-1 text-xs text-muted-foreground"
            >
              Sudah direkonsiliasi
            </div>
          </template>
          <template #cell-tercatat-roster="{ item }">
            {{ item.recorded_count }} / {{ item.roster_count }}
          </template>
          <template #cell-tindakan="{ item }">
            <TableActions
              :label="item.class_name"
              :actions="[
                { label: 'Buka detail', icon: 'view', to: `/admin/attendance/${item.id}` },
              ]"
            />
          </template>
          <template #empty>
            <Empty>
              <EmptyHeader>
                <EmptyTitle>Belum ada sesi pada tanggal ini</EmptyTitle>
                <EmptyDescription>
                  Periksa jadwal kelas dan kalender, atau buat sesi dari jadwal aktif.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </template>
        </DataTable>
      </CardContent>
      <CardFooter class="block border-t">
        <TablePagination
          :page="page"
          :total="total"
          label="sesi"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardFooter>
    </Card>
    <Dialog
      :open="open"
      @update:open="
        (value) => {
          if (!saving) open = value
        }
      "
    >
      <DialogContent
        :show-close-button="!saving"
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>Buat sesi absensi</DialogTitle>
          <DialogDescription>
            Sesi mengikuti jadwal dan kalender. Daftar siswa dibekukan saat sesi dibuka.
          </DialogDescription>
        </DialogHeader>
        <form
          novalidate
          class="flex flex-col gap-6"
          @submit.prevent="save"
        >
          <Alert
            v-if="message"
            variant="destructive"
          >
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <FieldGroup>
            <Field :data-invalid="!!errors.schedule_id">
              <FieldLabel for="session-schedule">Jadwal</FieldLabel>
              <Select
                v-model="form.schedule_id"
                :disabled="saving"
              >
                <SelectTrigger
                  id="session-schedule"
                  :aria-invalid="!!errors.schedule_id"
                  :aria-describedby="errors.schedule_id ? 'session-schedule-error' : undefined"
                  class="w-full"
                >
                  <SelectValue placeholder="Pilih jadwal aktif" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="item in schedules"
                    :key="item.id"
                    :value="item.id"
                  >
                    {{ item.class_name }} · {{ item.name }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FieldDescription v-if="!schedules.length">
                Tambahkan jadwal aktif terlebih dahulu.
              </FieldDescription>
              <FieldError
                v-if="errors.schedule_id"
                id="session-schedule-error"
              >
                {{ errors.schedule_id }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.date">
              <FieldLabel for="new-session-date">Tanggal</FieldLabel>
              <Input
                id="new-session-date"
                v-model="form.date"
                :aria-invalid="!!errors.date"
                :aria-describedby="errors.date ? 'new-session-date-error' : undefined"
                type="date"
                required
                :disabled="saving"
              />
              <FieldError
                v-if="errors.date"
                id="new-session-date-error"
              >
                {{ errors.date }}
              </FieldError>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="saving"
              @click="open = false"
            >
              Batal
            </Button>
            <Button
              type="submit"
              :disabled="saving"
            >
              <Spinner v-if="saving" />
              Buat sesi
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>
