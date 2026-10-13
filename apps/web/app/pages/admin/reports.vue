<script setup lang="ts">
import { Download, RotateCcw } from '@lucide/vue'
import { attendanceLabels } from '~/types/attendance'
import type { AcademicYear, SchoolClass } from '~/types/master'
import type { AttendanceReport, ReportFilters, ReportRow, AttendanceTotals } from '~/types/reports'
definePageMeta({ layout: 'admin', middleware: ['auth', 'reports'] })
const { request, options, dateTime, canExport, defaultFilters, download } = useReports()
const route = useRoute()
const form = reactive(defaultFilters())
for (const key of Object.keys(form) as (keyof ReportFilters)[]) { if (typeof route.query[key] === 'string') form[key] = route.query[key] as string }
const applied = ref<ReportFilters | null>(null)
const years = ref<AcademicYear[]>([])
const classes = ref<SchoolClass[]>([])
const entries = ref<ReportRow[]>([])
const totals = ref<AttendanceTotals | null>(null)
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const exporting = ref(false)
const message = ref('')
const success = ref('')
const yearSelection = computed({ get: () => form.academic_year_id || 'all', set: value => { form.academic_year_id = value === 'all' ? '' : value; form.class_id = '' } })
const classSelection = computed({ get: () => form.class_id || 'all', set: value => { form.class_id = value === 'all' ? '' : value } })
const statusSelection = computed({ get: () => form.status || 'all', set: value => { form.status = value === 'all' ? '' : value } })
const classOptions = computed(() => classes.value.filter(item => !form.academic_year_id || item.academic_year_id === form.academic_year_id))
const dirty = computed(() => !!applied.value && JSON.stringify(form) !== JSON.stringify(applied.value))
async function load(filters: ReportFilters, newPage = 1) {
  loading.value = true; message.value = ''; success.value = ''
  try {
    const result = await request<AttendanceReport>('/reports/attendance', { query: { ...filters, page: newPage, page_size: 20 } })
    Object.assign(form, result.filters)
    entries.value = result.data; totals.value = result.totals; applied.value = result.filters; page.value = result.pagination.page; total.value = result.pagination.total
  }
  catch (error) { message.value = apiMessage(error); entries.value = []; totals.value = null; applied.value = null }
  finally { loading.value = false }
}
function reset() { Object.assign(form, defaultFilters()); void load({ ...form }) }
async function exportCSV() {
  if (!applied.value || exporting.value) return
  exporting.value = true; message.value = ''
  try { await download(applied.value); success.value = 'CSV siap diunduh. Ekspor seluruh hasil filter telah tercatat di audit.' }
  catch (error) { message.value = apiMessage(error) }
  finally { exporting.value = false }
}
async function changePage(value: number) { if (applied.value) await load(applied.value, value) }
try { [years.value, classes.value] = await Promise.all([options<AcademicYear>('/academic-years'), options<SchoolClass>('/classes')]) }
catch (error) { message.value = apiMessage(error) }
await load({ ...form })
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">Laporan kehadiran</h1><p class="mt-2 text-sm text-muted-foreground">Rekap dari roster sesi, termasuk siswa yang belum memiliki catatan.</p></div><Button v-if="canExport" :disabled="loading || exporting || !applied || dirty" @click="exportCSV"><Spinner v-if="exporting" /><Download v-else />Ekspor CSV</Button></div>
    <Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Card><CardHeader><CardTitle>Filter laporan</CardTitle><CardDescription>Maksimal 366 hari. CSV berisi seluruh hasil filter, hingga 10.000 baris.</CardDescription></CardHeader><CardContent><form class="space-y-4" @submit.prevent="load({ ...form })"><FieldGroup class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"><Field><FieldLabel for="report-from">Dari tanggal</FieldLabel><Input id="report-from" v-model="form.from" type="date" required :disabled="loading || exporting" /></Field><Field><FieldLabel for="report-to">Sampai tanggal</FieldLabel><Input id="report-to" v-model="form.to" type="date" required :min="form.from" :disabled="loading || exporting" /></Field><Field><FieldLabel for="report-year">Tahun ajaran</FieldLabel><Select v-model="yearSelection" :disabled="loading || exporting"><SelectTrigger id="report-year" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua tahun ajaran</SelectItem><SelectItem v-for="item in years" :key="item.id" :value="item.id">{{ item.name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="report-class">Kelas</FieldLabel><Select v-model="classSelection" :disabled="loading || exporting"><SelectTrigger id="report-class" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua kelas</SelectItem><SelectItem v-for="item in classOptions" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="report-status">Status kehadiran</FieldLabel><Select v-model="statusSelection" :disabled="loading || exporting"><SelectTrigger id="report-status" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua status</SelectItem><SelectItem v-for="(label, key) in attendanceLabels" :key="key" :value="key">{{ label }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="report-search">Cari siswa</FieldLabel><Input id="report-search" v-model="form.search" placeholder="Nama atau NIS" maxlength="100" :disabled="loading || exporting" /></Field></FieldGroup><div class="flex flex-wrap items-center gap-3"><Button type="submit" :disabled="loading || exporting"><Spinner v-if="loading" />Terapkan filter</Button><Button type="button" variant="outline" :disabled="loading || exporting" @click="reset"><RotateCcw />Reset</Button><Badge v-if="form.student_id" variant="outline">Riwayat satu siswa</Badge><span v-if="dirty" class="text-sm text-muted-foreground">Terapkan perubahan filter sebelum mengekspor.</span></div></form></CardContent></Card>
    <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" role="status"><span class="sr-only">Memuat laporan</span><Skeleton v-for="n in 4" :key="n" class="h-36" /></div>
    <SectionCards v-else-if="totals" :totals="totals" />
    <Card class="min-w-0"><CardHeader><CardTitle>Rincian roster</CardTitle><CardDescription v-if="applied">{{ applied.from }} – {{ applied.to }} · {{ total }} baris. Satu baris untuk satu siswa pada satu sesi.</CardDescription><CardDescription v-else>Terapkan filter untuk melihat rincian.</CardDescription></CardHeader><CardContent class="min-w-0"><Table class="min-w-[980px]"><TableCaption class="sr-only">Laporan kehadiran sesuai filter</TableCaption><TableHeader><TableRow><TableHead>Tanggal / sesi</TableHead><TableHead>Siswa</TableHead><TableHead>Kelas / tahun</TableHead><TableHead>Status</TableHead><TableHead>Waktu hadir</TableHead><TableHead>Alasan</TableHead><TableHead class="text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in entries" :key="`${item.session_id}-${item.student_id}`"><TableCell>{{ item.date }}<div class="text-xs text-muted-foreground">{{ item.session_name }}</div></TableCell><TableCell class="font-medium">{{ item.full_name }}<div class="text-xs font-normal text-muted-foreground">{{ item.nis }}</div></TableCell><TableCell>{{ item.class_name }}<div class="text-xs text-muted-foreground">{{ item.academic_year_name }}</div></TableCell><TableCell><Badge :variant="['present', 'late'].includes(item.status) ? 'secondary' : 'outline'">{{ attendanceLabels[item.status] }}</Badge></TableCell><TableCell class="whitespace-nowrap">{{ item.check_in_at ? dateTime(item.check_in_at) : '—' }}</TableCell><TableCell class="max-w-48 whitespace-normal">{{ item.reason ?? '—' }}</TableCell><TableCell class="text-right"><Button as-child variant="ghost" size="sm"><NuxtLink :to="`/admin/attendance/${item.session_id}`">Lihat sesi</NuxtLink></Button></TableCell></TableRow><TableRow v-if="!entries.length && !loading"><TableCell colspan="7"><Empty><EmptyHeader><EmptyTitle>Tidak ada roster sesuai filter</EmptyTitle><EmptyDescription>Periksa tanggal dan kelas. Roster tersedia setelah sesi dibuka; sesi dibatalkan tidak masuk laporan.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody></Table></CardContent><CardFooter class="block border-t"><TablePagination :page="page" :total="total" label="baris" :disabled="loading || exporting" @update:page="changePage" /></CardFooter></Card>
  </div>
</template>
