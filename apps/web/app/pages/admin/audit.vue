<script setup lang="ts">
import type { Result } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'super-admin'] })
interface Entry { id: number, actor_id: string | null, action: string, target_type: string, target_id: string | null, request_id: string, occurred_at: string }
const request = useApi()
const auth = useAuth()
const entries = ref<Entry[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const message = ref('')
const labels: Record<string, string> = { 'attendance.schedules.created': 'Jadwal dibuat', 'attendance.schedules.updated': 'Jadwal diperbarui', 'attendance.calendar.updated': 'Kalender diperbarui', 'attendance.sessions.created': 'Sesi dibuat', 'attendance.sessions.open': 'Sesi dibuka', 'attendance.sessions.close': 'Sesi ditutup', 'attendance.sessions.cancel': 'Sesi dibatalkan', 'attendance.sessions.reconcile': 'Sesi direkonsiliasi', 'attendance.manual.recorded': 'Absensi manual dicatat', 'attendance.roster.added': 'Roster ditambah', 'attendance.corrections.requested': 'Koreksi diajukan', 'attendance.corrections.approved': 'Koreksi disetujui', 'attendance.corrections.rejected': 'Koreksi ditolak', 'auth.bootstrap': 'Admin pertama dibuat', 'auth.login': 'Masuk', 'auth.login_failed': 'Percobaan masuk gagal', 'auth.logout': 'Keluar', 'users.created': 'Pengguna dibuat', 'users.updated': 'Pengguna diperbarui', 'students.created': 'Siswa ditambahkan', 'students.updated': 'Siswa diperbarui', 'students.placed': 'Siswa ditempatkan', 'academics.years.created': 'Tahun ajaran dibuat', 'academics.years.updated': 'Tahun ajaran diperbarui', 'academics.classes.created': 'Kelas dibuat', 'academics.classes.updated': 'Kelas diperbarui', 'academics.assignments.created': 'Guru ditugaskan', 'academics.assignments.revoked': 'Penugasan dicabut' }
const date = (value: string) => new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short', timeZone: auth.session.value?.school.timezone ?? 'Asia/Jakarta' }).format(new Date(value))
async function load() {
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<Entry[]>>('/audit-logs', { query: { page: page.value, page_size: 20 } })
    entries.value = result.data
    total.value = result.pagination?.total ?? 0
  }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function changePage(value: number) { page.value = value; await load() }
await load()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Audit aktivitas</h1><p class="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">Histori perubahan akun, data akademik, siswa, dan sesi, tanpa password atau data biometrik.</p></div>
    <Alert v-if="message" variant="destructive" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <Card class="min-w-0 shadow-none">
      <CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle>Aktivitas sekolah</CardTitle><CardDescription class="mt-1.5">Catatan terbaru ditampilkan lebih dahulu.</CardDescription></div><Button variant="outline" :disabled="loading" @click="load"><Spinner v-if="loading" />Muat ulang</Button></CardHeader>
      <CardContent class="min-w-0">
        <div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat aktivitas…</span><Skeleton v-for="row in 5" :key="row" class="h-10 w-full" /></div>
        <Table v-else class="min-w-[760px]">
          <TableCaption class="sr-only">Audit aktivitas sekolah</TableCaption>
          <TableHeader><TableRow><TableHead>Waktu</TableHead><TableHead>Aktivitas</TableHead><TableHead>Pelaku</TableHead><TableHead>Target</TableHead><TableHead>Request ID</TableHead></TableRow></TableHeader>
          <TableBody><TableRow v-for="entry in entries" :key="entry.id"><TableCell class="whitespace-nowrap tabular-nums">{{ date(entry.occurred_at) }}</TableCell><TableCell>{{ labels[entry.action] ?? entry.action }}</TableCell><TableCell>{{ entry.actor_id?.slice(0, 8) ?? 'Anonim' }}</TableCell><TableCell>{{ entry.target_id?.slice(0, 8) ?? '—' }}</TableCell><TableCell class="font-mono text-xs">{{ entry.request_id }}</TableCell></TableRow><TableRow v-if="!entries.length"><TableCell colspan="5"><Empty><EmptyHeader><EmptyTitle>Belum ada aktivitas tercatat</EmptyTitle><EmptyDescription>Aktivitas login dan perubahan pengguna akan tampil di sini.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody>
        </Table>
      </CardContent>
      <CardFooter class="block border-t pt-4"><TablePagination :page="page" :total="total" label="aktivitas" :disabled="loading" @update:page="changePage" /></CardFooter>
    </Card>
  </div>
</template>
