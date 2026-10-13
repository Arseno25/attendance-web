<script setup lang="ts">
import { ArrowLeft, Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { attendanceLabels, sessionLabels, type AttendanceSession, type Participant } from '~/types/attendance'
definePageMeta({ layout: 'admin', middleware: ['auth', 'attendance'] })
const { request, options, dateTime, canManual, canApprove, canRequest } = useAttendance()
const route = useRoute()
const session = ref<AttendanceSession | null>(null)
const participants = ref<Participant[]>([])
const candidates = ref<{ id: string, full_name: string, nis: string }[]>([])
const page = ref(1)
const total = ref(0)
const search = ref('')
const statusFilter = ref('all')
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const success = ref('')
const sheet = ref('')
const selected = ref<Participant | null>(null)
const action = ref('')
const reason = ref('')
let originalLocal = ''
const form = reactive({ student_id: '', status: 'present', reason: '', check_in_local: '', event_id: '' })
const actions: Record<string, string> = { open: 'Buka sesi', close: 'Tutup sesi', cancel: 'Batalkan sesi', reconcile: 'Rekonsiliasi sesi' }
async function load() {
  loading.value = true; message.value = ''
  try {
    const [detail, roster] = await Promise.all([request<Result<AttendanceSession>>(`/attendance-sessions/${route.params.id}`), request<Result<Participant[]>>(`/attendance-sessions/${route.params.id}/participants`, { query: { page: page.value, page_size: 20, search: search.value, status: statusFilter.value === 'all' ? '' : statusFilter.value } })])
    session.value = detail.data; participants.value = roster.data; total.value = roster.pagination?.total ?? 0
  }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
function show(kind: string, participant: Participant) {
  message.value = ''; selected.value = participant
  let local = ''
  if (participant.check_in_at) local = new Intl.DateTimeFormat('sv-SE', { timeZone: session.value?.timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(new Date(participant.check_in_at)).replace(' ', 'T')
  originalLocal = local
  Object.assign(form, { student_id: participant.student_id, status: kind === 'manual' ? 'present' : participant.status === 'not_recorded' ? 'excused' : participant.status, reason: '', check_in_local: local, event_id: crypto.randomUUID() }); sheet.value = kind
}
async function addRoster() {
  message.value = ''; selected.value = null; Object.assign(form, { student_id: '', reason: '' })
  try { candidates.value = await options<{ id: string, full_name: string, nis: string }>(`/attendance-sessions/${route.params.id}/eligible-students`); sheet.value = 'add' }
  catch (error) { message.value = apiMessage(error) }
}
async function saveEntry() {
  saving.value = true; message.value = ''
  try {
    const session_id = String(route.params.id)
    if (sheet.value === 'add') { await request(`/attendance-sessions/${session_id}/participants`, { method: 'POST', body: { student_id: form.student_id, reason: form.reason } }); success.value = 'Siswa ditambahkan ke roster dengan alasan tercatat.' }
    else if (sheet.value === 'checkout') { await request('/attendances/check-out', { method: 'POST', body: { session_id, student_id: form.student_id, event_id: form.event_id, reason: form.reason } }); success.value = 'Waktu pulang tersimpan; waktu hadir pertama tetap dipertahankan.' }
    else if (sheet.value === 'manual') { const data = (await request<Result<{ outcome: string, record: { status: string } }>>('/attendances/manual', { method: 'POST', body: { session_id, student_id: form.student_id, event_id: form.event_id, status: form.status, reason: form.reason } })).data; success.value = data.outcome === 'already_recorded' ? 'Kehadiran sudah tercatat; waktu pertama dipertahankan.' : `Tersimpan: ${attendanceLabels[data.record.status]}.` }
    else { await request('/attendance-corrections', { method: 'POST', body: { session_id, student_id: form.student_id, status: form.status, reason: form.reason, ...(['present', 'late'].includes(form.status) ? (form.check_in_local === originalLocal && selected.value?.check_in_at ? { check_in_at: selected.value.check_in_at } : { check_in_local: form.check_in_local }) : {}) } }); success.value = 'Koreksi diajukan. Petugas yang berbeda harus memutuskan permintaan ini.' }
    sheet.value = ''; await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
watch(() => [form.status, form.reason, form.student_id], () => { if (['manual', 'checkout'].includes(sheet.value)) form.event_id = crypto.randomUUID() })
function confirm(value: string) { message.value = ''; reason.value = ''; action.value = value }
async function performAction() {
  saving.value = true; message.value = ''
  try { await request(`/attendance-sessions/${route.params.id}/${action.value}`, { method: 'POST', body: { reason: reason.value } }); action.value = ''; success.value = 'Status sesi berhasil diperbarui.'; await load() }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function filter() { page.value = 1; await load() }
async function changePage(value: number) { page.value = value; await load() }
const liveBlocked = computed(() => saving.value || !!sheet.value || !!action.value)
const liveState = useAttendanceLive(String(route.params.id), load, liveBlocked)
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6"><div><Button as-child variant="ghost" size="sm" class="-ml-3"><NuxtLink to="/admin/attendance"><ArrowLeft />Daftar sesi</NuxtLink></Button></div><Alert v-if="message && !sheet && !action" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <template v-if="session"><div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">{{ session.class_name }}</h1><p class="mt-2 text-sm text-muted-foreground">{{ session.name }} · {{ session.date }} · {{ session.timezone }}</p></div><div class="flex flex-wrap gap-2"><Button v-if="canManual && session.status === 'scheduled'" @click="confirm('open')">Buka sesi</Button><Button v-if="canManual && session.status === 'scheduled'" variant="outline" @click="confirm('cancel')">Batalkan sesi</Button><Button v-if="canManual && session.status === 'open'" variant="outline" @click="confirm('close')">Tutup sesi</Button><Button v-if="canApprove && session.status === 'closed' && !session.reconciled_at" @click="confirm('reconcile')">Rekonsiliasi</Button></div></div>
      <Card><CardHeader><CardTitle>Status sesi</CardTitle><p class="text-xs text-muted-foreground" role="status">{{ liveState }}</p><CardDescription>Belum tercatat berbeda dari alfa. Alfa hanya dibuat setelah peninjauan dan rekonsiliasi.</CardDescription></CardHeader><CardContent><dl class="grid gap-5 sm:grid-cols-2 xl:grid-cols-4"><div><dt class="text-sm text-muted-foreground">Status</dt><dd class="mt-1"><Badge :variant="session.status === 'open' ? 'secondary' : 'outline'">{{ sessionLabels[session.status] }}</Badge><span v-if="session.reconciled_at" class="ml-2 text-sm">Direkonsiliasi</span></dd></div><div><dt class="text-sm text-muted-foreground">Mulai</dt><dd class="mt-1 text-sm font-medium">{{ dateTime(session.starts_at) }}</dd></div><div><dt class="text-sm text-muted-foreground">Batas tepat waktu</dt><dd class="mt-1 text-sm font-medium">{{ dateTime(session.late_after) }}</dd></div><div><dt class="text-sm text-muted-foreground">Tutup terjadwal</dt><dd class="mt-1 text-sm font-medium">{{ dateTime(session.closes_at) }}</dd></div></dl></CardContent></Card>
      <Card class="min-w-0"><CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle>Roster siswa</CardTitle><CardDescription class="mt-1.5">{{ session.recorded_count }} dari {{ session.roster_count }} siswa memiliki catatan. Roster tetap saat siswa pindah kelas.</CardDescription></div><Button v-if="canManual && session.status === 'open'" variant="outline" @click="addRoster"><Plus />Tambah ke roster</Button></CardHeader><CardContent class="min-w-0 space-y-4"><form class="flex flex-wrap gap-3" @submit.prevent="filter"><Label for="roster-search" class="sr-only">Cari siswa</Label><Input id="roster-search" v-model="search" class="sm:max-w-xs" maxlength="100" placeholder="Cari nama atau NIS" /><Select v-model="statusFilter" :disabled="loading" @update:model-value="filter"><SelectTrigger aria-label="Filter status absensi" class="w-full sm:w-48"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">Semua status</SelectItem><SelectItem v-for="(label, key) in attendanceLabels" :key="key" :value="key">{{ label }}</SelectItem></SelectContent></Select><Button type="submit" variant="outline" :disabled="loading">Cari</Button></form><div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat roster</span><Skeleton v-for="n in 4" :key="n" class="h-10" /></div><Table v-else class="min-w-[720px]"><TableCaption class="sr-only">Roster dan catatan absensi siswa</TableCaption><TableHeader><TableRow><TableHead>Siswa</TableHead><TableHead>Status</TableHead><TableHead>Waktu hadir / pulang</TableHead><TableHead>Alasan</TableHead><TableHead v-if="canManual || canRequest" class="text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in participants" :key="item.id"><TableCell class="font-medium">{{ item.full_name }}<div class="text-xs font-normal text-muted-foreground">{{ item.nis }}</div></TableCell><TableCell><Badge :variant="['present', 'late'].includes(item.status) ? 'secondary' : 'outline'">{{ attendanceLabels[item.status] }}</Badge><div v-if="item.correction_pending" class="mt-1 text-xs text-muted-foreground">Koreksi menunggu</div></TableCell><TableCell>{{ item.check_in_at ? dateTime(item.check_in_at) : '—' }}<div v-if="item.check_out_at" class="text-xs text-muted-foreground">Pulang: {{ dateTime(item.check_out_at) }}</div></TableCell><TableCell class="max-w-48 whitespace-normal">{{ item.reason ?? '—' }}</TableCell><TableCell v-if="canManual || canRequest" class="text-right"><Button v-if="canManual && session.status === 'open' && item.status === 'not_recorded'" variant="ghost" size="sm" :disabled="item.correction_pending" @click="show('manual', item)">Catat</Button><Button v-if="canManual && session.status === 'open' && ['present', 'late'].includes(item.status) && !item.check_out_at" variant="ghost" size="sm" :disabled="item.correction_pending" @click="show('checkout', item)">Catat pulang</Button><Button v-if="canRequest && ['open', 'closed'].includes(session.status)" variant="ghost" size="sm" :disabled="item.correction_pending" @click="show('correction', item)">Koreksi</Button></TableCell></TableRow><TableRow v-if="!participants.length"><TableCell :colspan="canManual || canRequest ? 5 : 4"><Empty><EmptyHeader><EmptyTitle>Belum ada siswa sesuai filter</EmptyTitle><EmptyDescription>{{ session.status === 'scheduled' ? 'Roster tersedia setelah sesi dibuka.' : 'Siswa yang masuk setelah waktu mulai dapat ditambahkan dengan alasan.' }}</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody></Table></CardContent><CardFooter class="block border-t"><TablePagination :page="page" :total="total" label="siswa" :disabled="loading" @update:page="changePage" /></CardFooter></Card>
    </template>
    <Sheet :open="!!sheet" @update:open="value => { if (!value && !saving) sheet = '' }"><SheetContent class="overflow-y-auto"><SheetHeader><SheetTitle>{{ sheet === 'checkout' ? 'Catat pulang' : sheet === 'manual' ? 'Catat absensi manual' : sheet === 'add' ? 'Tambah siswa ke roster' : 'Ajukan koreksi' }}</SheetTitle><SheetDescription>{{ selected ? `${selected.full_name} · ${selected.nis}` : 'Siswa aktif yang belum termasuk roster sesi ini.' }} {{ sheet === 'checkout' ? 'Waktu pulang memakai jam server setelah verifikasi petugas.' : sheet === 'manual' ? 'Waktu hadir dan status terlambat dihitung server.' : sheet === 'correction' ? 'Catatan berubah setelah petugas berbeda menyetujui.' : 'Tambahan roster disimpan dengan alasan dan audit.' }}</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="saveEntry"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup><Field v-if="sheet === 'add'"><FieldLabel for="roster-student">Siswa</FieldLabel><Select v-model="form.student_id" :disabled="saving"><SelectTrigger id="roster-student" class="w-full"><SelectValue placeholder="Pilih siswa" /></SelectTrigger><SelectContent><SelectItem v-for="item in candidates" :key="item.id" :value="item.id">{{ item.full_name }} · {{ item.nis }}</SelectItem></SelectContent></Select><FieldDescription v-if="!candidates.length">Semua siswa aktif kelas ini sudah termasuk roster.</FieldDescription></Field><Field v-else-if="sheet !== 'checkout'"><FieldLabel for="entry-status">Status</FieldLabel><Select v-model="form.status" :disabled="saving"><SelectTrigger id="entry-status" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem v-for="key in sheet === 'manual' ? ['present', 'excused', 'sick'] : ['present', 'late', 'excused', 'sick', 'absent']" :key="key" :value="key">{{ attendanceLabels[key] }}</SelectItem></SelectContent></Select></Field><Field v-if="sheet === 'correction' && ['present', 'late'].includes(form.status)"><FieldLabel for="correction-time">Waktu hadir</FieldLabel><Input id="correction-time" v-model="form.check_in_local" type="datetime-local" step="1" required :disabled="saving" /><FieldDescription>Jam lokal {{ session?.timezone }}; status hadir/terlambat harus sesuai batas sesi.</FieldDescription></Field><Field><FieldLabel for="entry-reason">Alasan</FieldLabel><Textarea id="entry-reason" v-model="form.reason" required maxlength="500" :disabled="saving" placeholder="Hasil verifikasi administratif petugas" /></Field></FieldGroup><Button class="w-full" type="submit" :disabled="saving || !form.student_id"><Spinner v-if="saving" />{{ sheet === 'correction' ? 'Ajukan koreksi' : 'Simpan' }}</Button></form></SheetContent></Sheet>
    <Dialog :open="!!action" @update:open="value => { if (!value && !saving) action = '' }"><DialogContent><DialogHeader><DialogTitle>{{ actions[action] }}?</DialogTitle><DialogDescription>{{ action === 'reconcile' ? 'Siswa tanpa catatan akan menjadi alfa. Selesaikan koreksi pending dan tinjau roster terlebih dahulu; izin/sakit tetap dipertahankan.' : action === 'open' ? 'Roster siswa akan dibekukan berdasarkan penempatan pada waktu mulai jadwal.' : action === 'close' ? 'Pencatatan manual berhenti. Koreksi tetap dapat diajukan sebelum rekonsiliasi.' : 'Sesi yang belum dibuka dibatalkan tanpa membuat catatan alfa.' }}</DialogDescription></DialogHeader><form class="space-y-4" @submit.prevent="performAction"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Field><FieldLabel for="session-action-reason">Alasan tindakan</FieldLabel><Textarea id="session-action-reason" v-model="reason" required maxlength="500" :disabled="saving" /></Field><DialogFooter><Button variant="outline" type="button" :disabled="saving" @click="action = ''">Batal</Button><Button type="submit" :disabled="saving"><Spinner v-if="saving" />{{ actions[action] }}</Button></DialogFooter></form></DialogContent></Dialog>
  </div>
</template>
