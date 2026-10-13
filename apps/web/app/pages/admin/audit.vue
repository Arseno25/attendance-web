<script setup lang="ts">
import type { Result, User } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'super-admin'] })
interface Entry { id: number, actor_id: string | null, actor_name: string | null, action: string, target_type: string, target_id: string | null, request_id: string, occurred_at: string }
const request = useApi()
const auth = useAuth()
const entries = ref<Entry[]>([])
const actors = ref<User[]>([])
const filters = reactive({ from: '', to: '', actor_id: 'all', action: 'all', target_type: 'all' })
const targets: Record<string, string> = { user: 'Pengguna', session: 'Sesi akun', academic_year: 'Tahun ajaran', class: 'Kelas', student: 'Siswa', student_enrollment: 'Penempatan siswa', teacher_assignment: 'Penugasan guru', attendance_schedule: 'Jadwal absensi', school_calendar_day: 'Kalender', attendance_session: 'Sesi absensi', attendance_record: 'Catatan absensi', attendance_correction: 'Koreksi', attendance_report: 'Laporan' }
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const message = ref('')
const labels: Record<string, string> = { 'reports.attendance.exported': 'Laporan CSV diekspor', 'attendance.schedules.created': 'Jadwal dibuat', 'attendance.schedules.updated': 'Jadwal diperbarui', 'attendance.calendar.updated': 'Kalender diperbarui', 'attendance.sessions.created': 'Sesi dibuat', 'attendance.sessions.open': 'Sesi dibuka', 'attendance.sessions.close': 'Sesi ditutup', 'attendance.sessions.cancel': 'Sesi dibatalkan', 'attendance.sessions.reconcile': 'Sesi direkonsiliasi', 'attendance.manual.recorded': 'Absensi manual dicatat', 'attendance.roster.added': 'Roster ditambah', 'attendance.corrections.requested': 'Koreksi diajukan', 'attendance.corrections.approved': 'Koreksi disetujui', 'attendance.corrections.rejected': 'Koreksi ditolak', 'auth.bootstrap': 'Admin pertama dibuat', 'auth.login': 'Masuk', 'auth.login_failed': 'Percobaan masuk gagal', 'auth.logout': 'Keluar', 'users.created': 'Pengguna dibuat', 'users.updated': 'Pengguna diperbarui', 'students.created': 'Siswa ditambahkan', 'students.updated': 'Siswa diperbarui', 'students.placed': 'Siswa ditempatkan', 'academics.years.created': 'Tahun ajaran dibuat', 'academics.years.updated': 'Tahun ajaran diperbarui', 'academics.classes.created': 'Kelas dibuat', 'academics.classes.updated': 'Kelas diperbarui', 'academics.assignments.created': 'Guru ditugaskan', 'academics.assignments.revoked': 'Penugasan dicabut' }
const date = (value: string) => new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short', timeZone: auth.session.value?.school.timezone ?? 'Asia/Jakarta' }).format(new Date(value))
async function load() {
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<Entry[]>>('/audit-logs', { query: { page: page.value, page_size: 20, from: filters.from, to: filters.to, action: filters.action === 'all' ? '' : filters.action, actor_id: filters.actor_id === 'all' ? '' : filters.actor_id, target_type: filters.target_type === 'all' ? '' : filters.target_type } })
    entries.value = result.data
    total.value = result.pagination?.total ?? 0
  }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function changePage(value: number) { page.value = value; await load() }
async function filter() { page.value = 1; await load() }
function reset() { Object.assign(filters, { from: '', to: '', actor_id: 'all', action: 'all', target_type: 'all' }); void filter() }
try { actors.value = await useMasterData().options<User>('/users') }
catch (error) { message.value = apiMessage(error) }
await load()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Audit aktivitas</h1><p class="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">Histori perubahan akun, data akademik, siswa, dan sesi, tanpa password atau data biometrik.</p></div>
    <Alert v-if="message" variant="destructive" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <Card class="min-w-0 shadow-none">
      <CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle>Aktivitas sekolah</CardTitle><CardDescription class="mt-1.5">Catatan terbaru ditampilkan lebih dahulu.</CardDescription></div><Button variant="outline" :disabled="loading" @click="load"><Spinner v-if="loading" />Muat ulang</Button></CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <form class="space-y-4" @submit.prevent="filter"><FieldGroup class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5"><Field><FieldLabel for="audit-from">Dari tanggal</FieldLabel><Input id="audit-from" v-model="filters.from" type="date" :disabled="loading" /></Field><Field><FieldLabel for="audit-to">Sampai tanggal</FieldLabel><Input id="audit-to" v-model="filters.to" type="date" :min="filters.from" :disabled="loading" /></Field><Field><FieldLabel for="audit-actor">Pelaku</FieldLabel><Select v-model="filters.actor_id" :disabled="loading"><SelectTrigger id="audit-actor" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua pelaku</SelectItem><SelectItem v-for="actor in actors" :key="actor.id" :value="actor.id">{{ actor.full_name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="audit-action">Aktivitas</FieldLabel><Select v-model="filters.action" :disabled="loading"><SelectTrigger id="audit-action" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua aktivitas</SelectItem><SelectItem v-for="(label, value) in labels" :key="value" :value="value">{{ label }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="audit-target">Jenis target</FieldLabel><Select v-model="filters.target_type" :disabled="loading"><SelectTrigger id="audit-target" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua target</SelectItem><SelectItem v-for="(label, value) in targets" :key="value" :value="value">{{ label }}</SelectItem></SelectContent></Select></Field></FieldGroup><div class="flex gap-2"><Button type="submit" :disabled="loading">Terapkan filter</Button><Button type="button" variant="outline" :disabled="loading" @click="reset">Reset</Button></div></form>
        <div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat aktivitas…</span><Skeleton v-for="row in 5" :key="row" class="h-10 w-full" /></div>
        <Table v-else class="min-w-[760px]">
          <TableCaption class="sr-only">Audit aktivitas sekolah</TableCaption>
          <TableHeader><TableRow><TableHead>Waktu</TableHead><TableHead>Aktivitas</TableHead><TableHead>Pelaku</TableHead><TableHead>Target</TableHead><TableHead>Request ID</TableHead></TableRow></TableHeader>
          <TableBody><TableRow v-for="entry in entries" :key="entry.id"><TableCell class="whitespace-nowrap tabular-nums">{{ date(entry.occurred_at) }}</TableCell><TableCell>{{ labels[entry.action] ?? entry.action }}</TableCell><TableCell>{{ entry.actor_name ?? 'Sistem / anonim' }}</TableCell><TableCell>{{ targets[entry.target_type] ?? entry.target_type }}<div class="text-xs text-muted-foreground">{{ entry.target_id?.slice(0, 8) ?? '—' }}</div></TableCell><TableCell class="font-mono text-xs">{{ entry.request_id }}</TableCell></TableRow><TableRow v-if="!entries.length"><TableCell colspan="5"><Empty><EmptyHeader><EmptyTitle>Belum ada aktivitas tercatat</EmptyTitle><EmptyDescription>Aktivitas login dan perubahan pengguna akan tampil di sini.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody>
        </Table>
      </CardContent>
      <CardFooter class="block border-t pt-4"><TablePagination :page="page" :total="total" label="aktivitas" :disabled="loading" @update:page="changePage" /></CardFooter>
    </Card>
  </div>
</template>
