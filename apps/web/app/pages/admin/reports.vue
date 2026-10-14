<script setup lang="ts">
import { toast } from 'vue-sonner'
import { reportSchema } from '~/lib/validation/reports'
import { statusVariant } from '~/lib/status'
import { Download, RotateCcw } from '@lucide/vue'
import { attendanceLabels } from '~/types/attendance'
import type { AcademicYear, SchoolClass } from '~/types/master'
import type {
  AttendanceReport,
  ReportFilters,
  ReportRow,
  AttendanceTotals,
  AttendanceDay,
} from '~/types/reports'
definePageMeta({ layout: 'admin', middleware: ['auth', 'reports'] })
const { request, options, dateTime, canExport, defaultFilters, download } = useReports()
const route = useRoute()
const form = reactive(defaultFilters())
const { errors, validate, clear } = useFormValidation(
  () => reportSchema,
  () => form,
)
for (const key of Object.keys(form) as (keyof ReportFilters)[]) {
  if (typeof route.query[key] === 'string') form[key] = route.query[key] as string
}
const applied = ref<ReportFilters | null>(null)
const years = ref<AcademicYear[]>([])
const classes = ref<SchoolClass[]>([])
const entries = ref<ReportRow[]>([])
const daily = ref<AttendanceDay[]>([])
const totals = ref<AttendanceTotals | null>(null)
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const exporting = ref(false)
const message = ref('')
const yearSelection = computed({
  get: () => form.academic_year_id || 'all',
  set: (value) => {
    form.academic_year_id = value === 'all' ? '' : value
    form.class_id = ''
  },
})
const classSelection = computed({
  get: () => form.class_id || 'all',
  set: (value) => {
    form.class_id = value === 'all' ? '' : value
  },
})
const statusSelection = computed({
  get: () => form.status || 'all',
  set: (value) => {
    form.status = value === 'all' ? '' : value
  },
})
const classOptions = computed(() =>
  classes.value.filter(
    (item) => !form.academic_year_id || item.academic_year_id === form.academic_year_id,
  ),
)
const dirty = computed(
  () => !!applied.value && JSON.stringify(form) !== JSON.stringify(applied.value),
)
async function load(filters: ReportFilters, newPage = 1) {
  if (!reportSchema.safeParse(filters).success) {
    validate()
    return
  }
  loading.value = true
  message.value = ''

  try {
    const result = await request<AttendanceReport>('/reports/attendance', {
      query: { ...filters, page: newPage, page_size: 20 },
    })
    Object.assign(form, result.filters)
    entries.value = result.data
    daily.value = result.daily
    totals.value = result.totals
    applied.value = result.filters
    page.value = result.pagination.page
    total.value = result.pagination.total
  } catch (error) {
    message.value = apiMessage(error)
    entries.value = []
    daily.value = []
    totals.value = null
    applied.value = null
  } finally {
    loading.value = false
  }
}
function reset() {
  clear()
  Object.assign(form, defaultFilters())
  void load({ ...form })
}
async function exportCSV() {
  if (!applied.value || exporting.value) return
  exporting.value = true
  message.value = ''
  try {
    await download(applied.value)
    toast.success('Laporan siap diunduh.')
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    exporting.value = false
  }
}
async function changePage(value: number) {
  if (applied.value) await load(applied.value, value)
}
try {
  ;[years.value, classes.value] = await Promise.all([
    options<AcademicYear>('/academic-years'),
    options<SchoolClass>('/classes'),
  ])
} catch (error) {
  message.value = apiMessage(error)
}
await load({ ...form })
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Laporan kehadiran"
      description="Rekap dari daftar siswa sesi, termasuk siswa yang belum memiliki catatan."
    >
      <template #actions>
        <Button
          v-if="canExport"
          :disabled="loading || exporting || !applied || dirty"
          @click="exportCSV"
        >
          <Spinner v-if="exporting" />
          <Download v-else />
          Ekspor CSV
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="message"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Card>
      <CardHeader>
        <CardTitle>Filter laporan</CardTitle>
        <CardDescription>
          Maksimal 366 hari. CSV berisi seluruh hasil filter, hingga 10.000 baris.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          novalidate
          aria-label="Filter laporan"
          class="space-y-4"
          @submit.prevent="load({ ...form })"
        >
          <FieldError v-if="errors.student_id || errors.session_id">
            {{ errors.student_id || errors.session_id }}
          </FieldError>
          <FieldGroup class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <Field :data-invalid="!!errors.from">
              <FieldLabel for="report-from">Dari tanggal</FieldLabel>
              <Input
                id="report-from"
                v-model="form.from"
                :aria-invalid="!!errors.from"
                :aria-describedby="errors.from ? 'report-from-error' : undefined"
                type="date"
                required
                :disabled="loading || exporting"
              />
              <FieldError
                v-if="errors.from"
                id="report-from-error"
              >
                {{ errors.from }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.to">
              <FieldLabel for="report-to">Sampai tanggal</FieldLabel>
              <Input
                id="report-to"
                v-model="form.to"
                :aria-invalid="!!errors.to"
                :aria-describedby="errors.to ? 'report-to-error' : undefined"
                type="date"
                required
                :min="form.from"
                :disabled="loading || exporting"
              />
              <FieldError
                v-if="errors.to"
                id="report-to-error"
              >
                {{ errors.to }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.academic_year_id">
              <FieldLabel for="report-year">Tahun ajaran</FieldLabel>
              <Select
                v-model="yearSelection"
                :disabled="loading || exporting"
              >
                <SelectTrigger
                  id="report-year"
                  :aria-invalid="!!errors.academic_year_id"
                  :aria-describedby="errors.academic_year_id ? 'report-year-error' : undefined"
                  class="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua tahun ajaran</SelectItem>
                  <SelectItem
                    v-for="item in years"
                    :key="item.id"
                    :value="item.id"
                  >
                    {{ item.name }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FieldError
                v-if="errors.academic_year_id"
                id="report-year-error"
              >
                {{ errors.academic_year_id }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.class_id">
              <FieldLabel for="report-class">Kelas</FieldLabel>
              <Select
                v-model="classSelection"
                :disabled="loading || exporting"
              >
                <SelectTrigger
                  id="report-class"
                  :aria-invalid="!!errors.class_id"
                  :aria-describedby="errors.class_id ? 'report-class-error' : undefined"
                  class="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua kelas</SelectItem>
                  <SelectItem
                    v-for="item in classOptions"
                    :key="item.id"
                    :value="item.id"
                  >
                    {{ item.name }} · {{ item.academic_year_name }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FieldError
                v-if="errors.class_id"
                id="report-class-error"
              >
                {{ errors.class_id }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.status">
              <FieldLabel for="report-status">Status kehadiran</FieldLabel>
              <Select
                v-model="statusSelection"
                :disabled="loading || exporting"
              >
                <SelectTrigger
                  id="report-status"
                  :aria-invalid="!!errors.status"
                  :aria-describedby="errors.status ? 'report-status-error' : undefined"
                  class="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua status</SelectItem>
                  <SelectItem
                    v-for="(label, key) in attendanceLabels"
                    :key="key"
                    :value="key"
                  >
                    {{ label }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FieldError
                v-if="errors.status"
                id="report-status-error"
              >
                {{ errors.status }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.search">
              <FieldLabel for="report-search">Cari siswa</FieldLabel>
              <Input
                id="report-search"
                v-model="form.search"
                :aria-invalid="!!errors.search"
                :aria-describedby="errors.search ? 'report-search-error' : undefined"
                placeholder="Nama atau NIS"
                maxlength="100"
                :disabled="loading || exporting"
              />
              <FieldError
                v-if="errors.search"
                id="report-search-error"
              >
                {{ errors.search }}
              </FieldError>
            </Field>
          </FieldGroup>
          <div class="flex flex-wrap items-center justify-end gap-3">
            <Badge
              v-if="form.student_id"
              variant="outline"
            >
              Riwayat satu siswa
            </Badge>
            <span
              v-if="dirty"
              class="text-sm text-muted-foreground"
            >
              Terapkan perubahan filter sebelum mengekspor.
            </span>
            <div class="ml-auto flex gap-2">
              <Button
                type="submit"
                :disabled="loading || exporting"
              >
                <Spinner v-if="loading" />
                Terapkan filter
              </Button>
              <Button
                type="button"
                variant="outline"
                :disabled="loading || exporting"
                @click="reset"
              >
                <RotateCcw />
                Reset
              </Button>
            </div>
          </div>
        </form>
      </CardContent>
    </Card>
    <div
      v-if="loading"
      class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
      role="status"
    >
      <span class="sr-only">Memuat laporan</span>
      <Skeleton
        v-for="n in 4"
        :key="n"
        class="h-36"
      />
    </div>
    <SectionCards
      v-else-if="totals"
      :totals="totals"
    />
    <Card
      v-if="applied && !loading"
      class="min-w-0"
    >
      <CardHeader>
        <CardTitle>Rekap harian</CardTitle>
        <CardDescription>
          {{ applied.from }} – {{ applied.to }}. Rekap seluruh hasil filter, termasuk hari tanpa
          daftar siswa.
        </CardDescription>
      </CardHeader>
      <CardContent class="min-w-0">
        <DataTable
          class="min-w-[640px]"
          :data="daily ?? []"
          :columns="[
            { id: 'tanggal', label: 'Tanggal', cellClass: 'font-medium' },
            {
              id: 'roster',
              label: 'Roster',
              headClass: 'text-right',
              cellClass: 'text-right tabular-nums',
            },
            {
              id: 'sudah-hadir',
              label: 'Sudah hadir',
              headClass: 'text-right',
              cellClass: 'text-right tabular-nums',
            },
            {
              id: 'terlambat',
              label: 'Terlambat',
              headClass: 'text-right',
              cellClass: 'text-right tabular-nums',
            },
            {
              id: 'belum-tercatat',
              label: 'Belum tercatat',
              headClass: 'text-right',
              cellClass: 'text-right tabular-nums',
            },
            {
              id: 'rasio-hadir',
              label: 'Rasio hadir',
              headClass: 'text-right',
              cellClass: 'text-right tabular-nums',
            },
          ]"
          caption="Rekap kehadiran harian sesuai filter laporan"
          row-key="date"
          :loading="loading"
        >
          <template #cell-tanggal="{ item: day }">{{ day.date }}</template>
          <template #cell-roster="{ item: day }">{{ day.roster_total }}</template>
          <template #cell-sudah-hadir="{ item: day }">{{ day.attended }}</template>
          <template #cell-terlambat="{ item: day }">{{ day.late }}</template>
          <template #cell-belum-tercatat="{ item: day }">{{ day.not_recorded }}</template>
          <template #cell-rasio-hadir="{ item: day }">{{ day.attendance_rate }}%</template>
        </DataTable>
      </CardContent>
    </Card>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Rincian daftar siswa</CardTitle>
        <CardDescription v-if="applied">
          {{ applied.from }} – {{ applied.to }} · {{ total }} baris. Satu baris untuk satu siswa
          pada satu sesi.
        </CardDescription>
        <CardDescription v-else>Terapkan filter untuk melihat rincian.</CardDescription>
      </CardHeader>
      <CardContent class="min-w-0">
        <DataTable
          class="min-w-[980px]"
          :data="entries ?? []"
          :columns="[
            { id: 'tanggal-sesi', label: 'Tanggal / sesi' },
            { id: 'siswa', label: 'Siswa', cellClass: 'font-medium' },
            { id: 'kelas-tahun', label: 'Kelas / tahun' },
            { id: 'status', label: 'Status' },
            {
              id: 'waktu-hadir-pulang',
              label: 'Waktu hadir / pulang',
              cellClass: 'whitespace-nowrap',
            },
            { id: 'alasan', label: 'Alasan', cellClass: 'max-w-48 whitespace-normal' },
            {
              id: 'tindakan',
              label: 'Tindakan',
              headClass: 'w-24 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Laporan kehadiran sesuai filter"
          :loading="loading"
        >
          <template #cell-tanggal-sesi="{ item }">
            {{ item.date }}
            <div class="text-xs text-muted-foreground">{{ item.session_name }}</div>
          </template>
          <template #cell-siswa="{ item }">
            {{ item.full_name }}
            <div class="text-xs font-normal text-muted-foreground">
              {{ item.nis }}
            </div>
          </template>
          <template #cell-kelas-tahun="{ item }">
            {{ item.class_name }}
            <div class="text-xs text-muted-foreground">
              {{ item.academic_year_name }}
            </div>
          </template>
          <template #cell-status="{ item }">
            <Badge :variant="statusVariant(item.status)">
              {{ attendanceLabels[item.status] }}
            </Badge>
          </template>
          <template #cell-waktu-hadir-pulang="{ item }">
            {{ item.check_in_at ? dateTime(item.check_in_at) : '—' }}
            <div
              v-if="item.check_out_at"
              class="text-xs text-muted-foreground"
            >
              Pulang: {{ dateTime(item.check_out_at) }}
            </div>
          </template>
          <template #cell-alasan="{ item }">{{ item.reason ?? '—' }}</template>
          <template #cell-tindakan="{ item }">
            <TableActions
              :label="item.full_name"
              :actions="[
                {
                  label: 'Lihat sesi',
                  icon: 'view',
                  to: `/admin/attendance/${item.session_id}`,
                },
              ]"
            />
          </template>
          <template #empty>
            <Empty>
              <EmptyHeader>
                <EmptyTitle>Tidak ada daftar siswa sesuai filter</EmptyTitle>
                <EmptyDescription>
                  Periksa tanggal dan kelas. Daftar siswa tersedia setelah sesi dibuka; sesi
                  dibatalkan tidak masuk laporan.
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
          label="baris"
          :disabled="loading || exporting"
          @update:page="changePage"
        />
      </CardFooter>
    </Card>
  </div>
</template>
