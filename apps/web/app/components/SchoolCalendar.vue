<script setup lang="ts">
import { parseDate, type DateValue } from '@internationalized/date'
import { useEventListener, useIntervalFn } from '@vueuse/core'
import { toast } from 'vue-sonner'
import { calendarRangeSchema } from '~/lib/validation/attendance'
import type { CalendarDay, SchoolCalendarMonth } from '~/types/attendance'
import type { Result } from '~/types/auth'

const props = defineProps<{ canEdit: boolean }>()
const { request, today, dateTime } = useAttendance()
const currentDay = ref(today())
const selected = shallowRef<DateValue>(parseDate(currentDay.value))
const placeholder = shallowRef<DateValue>(selected.value)
const month = computed(() => placeholder.value.toString().slice(0, 7))
const data = ref<SchoolCalendarMonth | null>(null)
const loading = ref(false)
const saving = ref(false)
const syncing = ref(false)
const message = ref('')
const open = ref(false)
const resetOpen = ref(false)
const form = reactive({
  from: currentDay.value,
  to: currentDay.value,
  is_school_day: false,
  reason: '',
})
const { errors, validate, clear } = useFormValidation(
  () => calendarRangeSchema,
  () => form,
)
const busy = computed(() => loading.value || saving.value || syncing.value)
const selectedDay = computed(() =>
  data.value?.days.find((day) => day.date === selected.value.toString()),
)
const events = computed(() => data.value?.days.filter((day) => day.source !== 'schedule') ?? [])
const byDate = computed(() => new Map(data.value?.days.map((day) => [day.date, day])))
const sourceLabels = {
  school: 'Pengaturan sekolah',
  national_holiday: 'Libur nasional',
  joint_leave: 'Cuti bersama',
  schedule: 'Jadwal kelas',
}
const dateTones: Partial<Record<CalendarDay['source'], string>> = {
  national_holiday: 'bg-danger text-danger-foreground',
  joint_leave: 'bg-warning text-warning-foreground',
  school: 'bg-info text-info-foreground',
}
let generation = 0
let controller: AbortController | undefined
let disposed = false
function dateLabel(value: string) {
  return new Intl.DateTimeFormat('id-ID', { dateStyle: 'long', timeZone: 'UTC' }).format(
    new Date(`${value}T00:00:00Z`),
  )
}
function dayLabel(value: string) {
  const day = byDate.value.get(value)
  return [dateLabel(value), day && sourceLabels[day.source], day?.reason]
    .filter(Boolean)
    .join(' · ')
}
function choose(value: DateValue | DateValue[] | undefined) {
  if (value && !Array.isArray(value)) selected.value = value
}
function goToToday() {
  currentDay.value = today()
  const date = parseDate(currentDay.value)
  selected.value = date
  placeholder.value = date
}
function refreshCurrentDay() {
  if (today() !== currentDay.value) goToToday()
}
useIntervalFn(refreshCurrentDay, 60_000)
onMounted(() => {
  refreshCurrentDay()
  useEventListener(window, 'focus', refreshCurrentDay)
  useEventListener(document, 'visibilitychange', refreshCurrentDay)
})
async function load() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  data.value = null
  message.value = ''
  try {
    const response = await request<Result<SchoolCalendarMonth>>('/school/calendar/month', {
      query: { month: month.value },
      signal: controller.signal,
    })
    if (!disposed && current === generation) data.value = response.data
  } catch (error) {
    if (!disposed && current === generation && !controller.signal.aborted)
      message.value = apiMessage(error)
  } finally {
    if (!disposed && current === generation) loading.value = false
  }
}
function show(day?: CalendarDay) {
  if (!props.canEdit || busy.value) return
  clear()
  const target = day?.date ?? selected.value.toString()
  Object.assign(form, {
    from: target,
    to: target,
    is_school_day: day?.is_school_day ?? false,
    reason: day?.is_override ? day.reason : '',
  })
  open.value = true
}
async function save() {
  if (!props.canEdit || saving.value || !validate()) return
  saving.value = true
  try {
    await request('/school/calendar', { method: 'PUT', body: { ...form } })
    open.value = false
    selected.value = parseDate(form.from)
    const nextMonth = form.from.slice(0, 7)
    if (month.value !== nextMonth) placeholder.value = parseDate(form.from)
    else await load()
    toast.success('Pengaturan kalender sekolah tersimpan.')
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function reset() {
  if (!props.canEdit || saving.value || !selectedDay.value?.is_override) return
  saving.value = true
  try {
    await request(`/school/calendar/${selected.value}`, { method: 'DELETE' })
    resetOpen.value = false
    toast.success('Tanggal kembali mengikuti kalender otomatis.')
    await load()
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function sync() {
  if (!props.canEdit || busy.value) return
  syncing.value = true
  try {
    await request('/school/calendar/sync', {
      method: 'POST',
      body: { year: placeholder.value.year },
    })
    toast.success('Libur nasional dan cuti bersama berhasil diperbarui.')
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    await load()
    syncing.value = false
  }
}
watch(
  month,
  () => {
    if (selected.value.toString().slice(0, 7) !== month.value)
      selected.value = parseDate(`${month.value}-01`)
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  disposed = true
  generation++
  controller?.abort()
})
defineExpose({ show, sync, busy })
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <Alert
      v-if="message"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Alert v-if="data && data.sync.status !== 'ready'">
      <AlertTitle>
        {{
          data.sync.synced_at
            ? 'Menggunakan data libur tersimpan'
            : `Data libur ${data.sync.year} belum tersedia`
        }}
      </AlertTitle>
      <AlertDescription>
        {{
          data.sync.synced_at
            ? 'Pembaruan terakhir belum berhasil. Pengaturan sekolah dan data yang tersimpan tetap berlaku.'
            : 'Perbarui data libur atau tetapkan hari sekolah sendiri. Sesi otomatis menunggu data kalender untuk tahun ini.'
        }}
      </AlertDescription>
    </Alert>
    <div class="grid min-w-0 gap-6">
      <Card class="min-w-0">
        <CardHeader>
          <CardTitle>Kalender sekolah</CardTitle>
          <CardDescription>Pilih tanggal untuk melihat pengaturan harinya.</CardDescription>
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              type="button"
              @click="goToToday"
            >
              Hari ini
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent class="flex min-w-0 flex-col gap-4 px-4 sm:px-6">
          <Calendar
            v-model:placeholder="placeholder"
            class="w-full p-0 [&_[data-slot=calendar-cell]]:min-w-0 [&_[data-slot=calendar-grid-row]]:gap-1"
            :model-value="selected"
            locale="id-ID"
            weekday-format="short"
            :week-starts-on="1"
            fixed-weeks
            prevent-deselect
            :min-value="parseDate('1900-01-01')"
            :max-value="parseDate('2200-12-31')"
            @update:model-value="choose"
          >
            <template #day="{ day, month: shownMonth }">
              <CalendarCellTrigger
                :day="day"
                :month="shownMonth"
                :data-today="day.toString() === currentDay ? '' : undefined"
                class="relative h-12 w-full flex-col items-start justify-start gap-1 overflow-hidden rounded-md border border-border/60 p-1 text-left sm:h-24 sm:p-2"
                :aria-label="dayLabel(day.toString())"
                :title="byDate.get(day.toString())?.reason"
              >
                <span
                  class="inline-flex size-5 max-w-full shrink-0 items-center justify-center rounded-sm text-sm leading-none font-medium tabular-nums sm:size-6"
                  :class="dateTones[byDate.get(day.toString())?.source ?? 'schedule']"
                >
                  {{ day.day }}
                </span>
                <template
                  v-if="
                    byDate.get(day.toString())?.source !== 'schedule' && byDate.has(day.toString())
                  "
                >
                  <span
                    class="hidden max-w-full text-xs leading-snug whitespace-normal wrap-anywhere sm:line-clamp-2"
                  >
                    {{ byDate.get(day.toString())?.reason }}
                  </span>
                </template>
              </CalendarCellTrigger>
            </template>
          </Calendar>
          <div class="flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
            <span class="flex items-center gap-2">
              <span class="size-1.5 rounded-full bg-danger-foreground" />
              Libur nasional
            </span>
            <span class="flex items-center gap-2">
              <span class="size-1.5 rounded-full bg-warning-foreground" />
              Cuti bersama
            </span>
            <span class="flex items-center gap-2">
              <span class="size-1.5 rounded-full bg-info-foreground" />
              Pengaturan sekolah
            </span>
          </div>
        </CardContent>
        <CardFooter class="flex flex-col items-start gap-1 border-t text-xs text-muted-foreground">
          <span v-if="data?.sync.synced_at">Diperbarui {{ dateTime(data.sync.synced_at) }}</span>
          <a
            href="https://indonesian-holidays.ahrulsyamil.com/docs"
            target="_blank"
            rel="noopener noreferrer"
            class="underline underline-offset-4"
          >
            Sumber libur Indonesia
          </a>
        </CardFooter>
      </Card>
      <Card class="min-w-0">
        <CardHeader>
          <CardTitle>{{ dateLabel(selected.toString()) }}</CardTitle>
          <CardDescription>
            Pengaturan sekolah diprioritaskan daripada kalender libur otomatis.
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <Skeleton
            v-if="loading"
            class="h-24"
          />
          <template v-else-if="selectedDay">
            <div class="flex flex-wrap gap-2">
              <Badge
                :variant="
                  selectedDay.is_school_day === false
                    ? 'warning'
                    : selectedDay.is_school_day
                      ? 'success'
                      : 'secondary'
                "
              >
                {{
                  selectedDay.is_school_day === false
                    ? 'Libur'
                    : selectedDay.is_school_day
                      ? 'Hari sekolah'
                      : 'Mengikuti jadwal kelas'
                }}
              </Badge>
              <Badge variant="outline">{{ sourceLabels[selectedDay.source] }}</Badge>
            </div>
            <p class="wrap-anywhere text-sm">{{ selectedDay.reason }}</p>
            <p
              v-if="selectedDay.is_override && selectedDay.holiday_name"
              class="text-sm text-muted-foreground"
            >
              Libur pada kalender nasional: {{ selectedDay.holiday_name }}.
            </p>
            <div
              v-if="canEdit && selectedDay.is_override"
              class="flex flex-wrap justify-end gap-2"
            >
              <Button
                variant="outline"
                :disabled="busy"
                @click="resetOpen = true"
              >
                Gunakan kalender otomatis
              </Button>
            </div>
          </template>
          <p class="text-sm text-muted-foreground">
            Libur berlaku untuk pembuatan dan pembukaan sesi. Sesi yang sudah berjalan tetap
            dipertahankan; tinjau sesi tersebut bila ada perubahan mendadak.
          </p>
        </CardContent>
      </Card>
    </div>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Libur dan pengaturan bulan ini</CardTitle>
        <CardDescription>
          Libur nasional, cuti bersama, dan pengaturan khusus sekolah.
        </CardDescription>
      </CardHeader>
      <CardContent class="min-w-0">
        <DataTable
          :data="events"
          :columns="[
            { id: 'date', label: 'Tanggal' },
            { id: 'reason', label: 'Keterangan' },
            { id: 'source', label: 'Sumber' },
            { id: 'status', label: 'Jenis hari' },
            {
              id: 'actions',
              label: 'Tindakan',
              headClass: 'w-16 text-right',
              cellClass: 'text-right',
              hidden: !canEdit,
            },
          ]"
          caption="Kalender libur bulan ini"
          class="min-w-[680px]"
        >
          <template #cell-date="{ item }">{{ dateLabel(item.date) }}</template>
          <template #cell-reason="{ item }">{{ item.reason }}</template>
          <template #cell-source="{ item }">
            <Badge variant="outline">{{ sourceLabels[item.source] }}</Badge>
          </template>
          <template #cell-status="{ item }">
            <Badge :variant="item.is_school_day ? 'success' : 'warning'">
              {{ item.is_school_day ? 'Hari sekolah' : 'Libur' }}
            </Badge>
          </template>
          <template #cell-actions="{ item }">
            <TableActions
              :label="dateLabel(item.date)"
              :disabled="busy"
              :actions="[{ label: 'Edit tanggal', icon: 'edit', onSelect: () => show(item) }]"
            />
          </template>
          <template #empty>
            <Empty>
              <EmptyHeader>
                <EmptyTitle>
                  {{ loading ? 'Memuat kalender' : 'Tidak ada libur atau pengaturan khusus' }}
                </EmptyTitle>
                <EmptyDescription>
                  Tanggal lain mengikuti hari aktif pada jadwal masing-masing kelas.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </template>
        </DataTable>
      </CardContent>
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
          <DialogTitle>Atur kalender sekolah</DialogTitle>
          <DialogDescription>
            Pilih satu tanggal atau rentang untuk libur semester, kegiatan sekolah, atau hari
            belajar pengganti.
          </DialogDescription>
        </DialogHeader>
        <form
          id="school-calendar-form"
          novalidate
          class="space-y-6"
          @submit.prevent="save"
        >
          <FieldGroup>
            <div class="grid gap-4 sm:grid-cols-2">
              <Field :data-invalid="!!errors.from">
                <FieldLabel for="calendar-from">Tanggal mulai</FieldLabel>
                <Input
                  id="calendar-from"
                  v-model="form.from"
                  type="date"
                  required
                  :disabled="saving"
                  :aria-invalid="!!errors.from"
                  :aria-describedby="errors.from ? 'calendar-from-error' : undefined"
                />
                <FieldError
                  v-if="errors.from"
                  id="calendar-from-error"
                >
                  {{ errors.from }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.to">
                <FieldLabel for="calendar-to">Tanggal selesai</FieldLabel>
                <Input
                  id="calendar-to"
                  v-model="form.to"
                  type="date"
                  required
                  :disabled="saving"
                  :aria-invalid="!!errors.to"
                  :aria-describedby="errors.to ? 'calendar-to-error' : undefined"
                />
                <FieldError
                  v-if="errors.to"
                  id="calendar-to-error"
                >
                  {{ errors.to }}
                </FieldError>
              </Field>
            </div>
            <Field orientation="horizontal">
              <Switch
                id="calendar-school-day"
                v-model="form.is_school_day"
                :disabled="saving"
              />
              <FieldContent>
                <FieldLabel for="calendar-school-day">Tetap masuk sekolah</FieldLabel>
                <FieldDescription>
                  {{
                    form.is_school_day
                      ? 'Tanggal ini menjadi hari sekolah, termasuk jika bertepatan dengan libur nasional.'
                      : 'Tanggal ini menjadi hari libur sekolah.'
                  }}
                </FieldDescription>
              </FieldContent>
            </Field>
            <Field :data-invalid="!!errors.reason">
              <FieldLabel for="calendar-reason">Keterangan</FieldLabel>
              <Textarea
                id="calendar-reason"
                v-model="form.reason"
                required
                maxlength="500"
                :disabled="saving"
                :aria-invalid="!!errors.reason"
                :aria-describedby="errors.reason ? 'calendar-reason-error' : undefined"
                placeholder="Contoh: Libur semester ganjil"
              />
              <FieldError
                v-if="errors.reason"
                id="calendar-reason-error"
              >
                {{ errors.reason }}
              </FieldError>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant="outline"
              type="button"
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
              Simpan pengaturan
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <AlertDialog
      :open="resetOpen"
      @update:open="
        (value) => {
          if (!saving) resetOpen = value
        }
      "
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Gunakan kalender otomatis?</AlertDialogTitle>
          <AlertDialogDescription>
            Pengaturan sekolah untuk {{ dateLabel(selected.toString()) }} akan dilepas. Tanggal ini
            kembali mengikuti libur nasional, cuti bersama, atau jadwal kelas.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="saving">Batal</AlertDialogCancel>
          <Button
            :disabled="saving"
            @click="reset"
          >
            <Spinner v-if="saving" />
            Gunakan kalender otomatis
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
