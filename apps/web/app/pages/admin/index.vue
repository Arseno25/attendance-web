<script setup lang="ts">
import { statusVariant } from '~/lib/status'
import { ClipboardCheck, Download, ScrollText, Users } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { AcademicYear, SchoolClass } from '~/types/master'
import type { AttendanceDashboard } from '~/types/reports'
import { attendanceLabels } from '~/types/attendance'
definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
const { request, options, today, dateTime, canExport, download } = useReports()
const canView = computed(() => ['super_admin', 'admin', 'teacher'].includes(auth.session.value?.user.role ?? ''))
const date = ref(today())
const year = ref('all')
const schoolClass = ref('all')
const years = ref<AcademicYear[]>([])
const classes = ref<SchoolClass[]>([])
const summary = ref<AttendanceDashboard | null>(null)
const loading = ref(false)
const exporting = ref(false)
const message = ref('')
const success = ref('')
const classOptions = computed(() => classes.value.filter(item => year.value === 'all' || item.academic_year_id === year.value))
const dirty = computed(() => !!summary.value && (date.value !== summary.value.filters.to || (year.value === 'all' ? '' : year.value) !== summary.value.filters.academic_year_id || (schoolClass.value === 'all' ? '' : schoolClass.value) !== summary.value.filters.class_id))
watch(year, () => { schoolClass.value = 'all' })
async function load() {
  if (!canView.value) return
  loading.value = true; message.value = ''; success.value = ''
  try { summary.value = (await request<Result<AttendanceDashboard>>('/dashboard/summary', { query: { date: date.value, academic_year_id: year.value === 'all' ? '' : year.value, class_id: schoolClass.value === 'all' ? '' : schoolClass.value } })).data }
  catch (error) { message.value = apiMessage(error); summary.value = null }
  finally { loading.value = false }
}
async function exportCSV() {
  if (!summary.value) return
  exporting.value = true; message.value = ''
  try { await download(summary.value.filters); success.value = 'CSV siap diunduh. Ekspor tercatat di audit.' }
  catch (error) { message.value = apiMessage(error) }
  finally { exporting.value = false }
}
if (canView.value) {
  try { [years.value, classes.value] = await Promise.all([options<AcademicYear>('/academic-years'), options<SchoolClass>('/classes')]) }
  catch (error) { message.value = apiMessage(error) }
  await load()
}
const maskedNIS = (value: string) => `•••${value.slice(-3)}`
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">Ringkasan kehadiran</h1><p class="mt-2 text-sm text-muted-foreground">Kehadiran dari roster sesi, sesuai tanggal dan kelas yang dapat Anda akses.</p></div><Button v-if="canView && canExport" :disabled="loading || exporting || !summary || dirty" @click="exportCSV"><Spinner v-if="exporting" /><Download v-else />Unduh CSV</Button></div>
    <Alert v-if="route.query.forbidden" variant="destructive"><AlertDescription>Halaman tersebut hanya tersedia untuk pengguna yang memiliki izin.</AlertDescription></Alert><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <template v-if="canView">
      <form class="grid gap-3 sm:grid-cols-2 xl:flex xl:items-end" @submit.prevent="load"><Field class="xl:w-48"><FieldLabel for="dashboard-date">Tanggal</FieldLabel><Input id="dashboard-date" v-model="date" type="date" required :disabled="loading || exporting" /></Field><Field class="xl:w-56"><FieldLabel for="dashboard-year">Tahun ajaran</FieldLabel><Select v-model="year" :disabled="loading || exporting"><SelectTrigger id="dashboard-year" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua tahun ajaran</SelectItem><SelectItem v-for="item in years" :key="item.id" :value="item.id">{{ item.name }}</SelectItem></SelectContent></Select></Field><Field class="xl:w-56"><FieldLabel for="dashboard-class">Kelas</FieldLabel><Select v-model="schoolClass" :disabled="loading || exporting"><SelectTrigger id="dashboard-class" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua kelas</SelectItem><SelectItem v-for="item in classOptions" :key="item.id" :value="item.id">{{ item.name }}</SelectItem></SelectContent></Select></Field><Button type="submit" :disabled="loading || exporting"><Spinner v-if="loading" />Tampilkan</Button></form>
      <p v-if="dirty" class="text-sm text-muted-foreground">Tampilkan perubahan filter sebelum mengunduh CSV.</p>
      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" role="status"><span class="sr-only">Memuat ringkasan</span><Skeleton v-for="n in 4" :key="n" class="h-36" /></div>
      <template v-else-if="summary"><SectionCards :totals="summary.totals" />
        <Card class="min-w-0"><CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle>Kehadiran tujuh hari</CardTitle><CardDescription class="mt-1.5">{{ summary.trend[0]?.date }} – {{ summary.filters.to }}. Terlambat termasuk sudah hadir; hari tanpa roster bukan alfa.</CardDescription></div><Button as-child variant="outline"><NuxtLink :to="{ path: '/admin/reports', query: { ...summary.filters } }">Lihat laporan</NuxtLink></Button></CardHeader><CardContent class="min-w-0"><Table class="min-w-[640px]"><TableCaption class="sr-only">Ringkasan kehadiran tujuh hari</TableCaption><TableHeader class="bg-muted/50"><TableRow><TableHead>Tanggal</TableHead><TableHead class="text-right">Roster</TableHead><TableHead class="text-right">Sudah hadir</TableHead><TableHead class="text-right">Terlambat</TableHead><TableHead class="text-right">Belum tercatat</TableHead><TableHead class="text-right">Rasio hadir</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="day in summary.trend" :key="day.date"><TableCell><NuxtLink :to="{ path: '/admin/reports', query: { ...summary.filters, from: day.date, to: day.date } }" class="font-medium hover:underline">{{ day.date }}</NuxtLink></TableCell><TableCell class="text-right tabular-nums">{{ day.roster_total }}</TableCell><TableCell class="text-right tabular-nums">{{ day.attended }}</TableCell><TableCell class="text-right tabular-nums">{{ day.late }}</TableCell><TableCell class="text-right tabular-nums">{{ day.not_recorded }}</TableCell><TableCell class="text-right tabular-nums">{{ day.attendance_rate }}%</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card class="min-w-0"><CardHeader><CardTitle>Catatan terbaru</CardTitle><CardDescription>Sepuluh catatan terakhir pada tanggal dan kelas yang dipilih, termasuk perubahan yang disetujui.</CardDescription></CardHeader><CardContent class="min-w-0"><Table class="min-w-[700px]"><TableCaption class="sr-only">Catatan kehadiran terbaru</TableCaption><TableHeader class="bg-muted/50"><TableRow><TableHead>Siswa</TableHead><TableHead>Kelas</TableHead><TableHead>Status</TableHead><TableHead>Waktu hadir</TableHead><TableHead class="w-12 text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in summary.recent" :key="`${item.session_id}-${item.student_id}`"><TableCell class="font-medium">{{ item.full_name }}<div class="text-xs font-normal text-muted-foreground">{{ maskedNIS(item.nis) }}</div></TableCell><TableCell>{{ item.class_name }}</TableCell><TableCell><Badge :variant="statusVariant(item.status)">{{ attendanceLabels[item.status] }}</Badge></TableCell><TableCell>{{ item.check_in_at ? dateTime(item.check_in_at) : '—' }}</TableCell><TableCell class="text-right"><TableActions :label="item.full_name"><DropdownMenuItem as-child><NuxtLink :to="`/admin/attendance/${item.session_id}`">Lihat sesi</NuxtLink></DropdownMenuItem></TableActions></TableCell></TableRow><TableRow v-if="!summary.recent.length"><TableCell colspan="5"><Empty><EmptyHeader><EmptyTitle>Belum ada catatan pada filter ini</EmptyTitle><EmptyDescription>Periksa sesi absensi dan roster. Siswa tanpa catatan tetap dihitung sebagai belum tercatat.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody></Table></CardContent></Card>
      </template>
    </template>
    <Card v-if="auth.session.value?.permissions.includes('attendance.read') || auth.session.value?.permissions.includes('attendance.manual')"><CardHeader><CardTitle>Pengelolaan absensi</CardTitle><CardDescription>Catat kehadiran dan tinjau koreksi. Alfa menunggu penutupan sesi dan rekonsiliasi.</CardDescription></CardHeader><CardContent class="flex flex-wrap gap-3"><Button as-child><NuxtLink to="/admin/attendance"><ClipboardCheck />Buka sesi absensi</NuxtLink></Button><Button as-child variant="outline"><NuxtLink to="/admin/corrections">Antrean koreksi</NuxtLink></Button><Button v-if="auth.session.value?.permissions.includes('academics.write')" as-child variant="outline"><NuxtLink to="/admin/students"><Users />Kelola siswa</NuxtLink></Button><Button v-if="auth.session.value?.user.role === 'super_admin'" as-child variant="outline"><NuxtLink to="/admin/audit"><ScrollText />Audit aktivitas</NuxtLink></Button></CardContent></Card>
  </div>
</template>
