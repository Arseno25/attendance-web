<script setup lang="ts">
import { statusVariant } from '~/lib/status'
import { Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { SchoolClass } from '~/types/master'
import type { Schedule, CalendarDay } from '~/types/attendance'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canConfigure, today } = useAttendance()
const tab = ref('schedules')
const schedules = ref<Schedule[]>([])
const classes = ref<SchoolClass[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const open = ref(false)
const editing = ref('')
const message = ref('')
const success = ref('')
const date = ref(today())
const calendar = reactive({ is_school_day: true, reason: '' })
const calendarInfo = ref('')
const form = reactive({ name: '', class_id: '', weekdays: [1, 2, 3, 4, 5], checkin_start: '07:00', checkin_cutoff: '07:30', grace_minutes: 0, close_time: '12:00', is_active: true })
const dayLabels = ['Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu', 'Minggu']
async function load() {
  loading.value = true; message.value = ''
  try { const result = await request<Result<Schedule[]>>('/attendance-schedules', { query: { page: page.value, page_size: 20 } }); schedules.value = result.data; total.value = result.pagination?.total ?? 0 }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function show(item?: Schedule) {
  message.value = ''; success.value = ''; editing.value = item?.id ?? ''
  Object.assign(form, { name: '', class_id: '', weekdays: [1, 2, 3, 4, 5], checkin_start: '07:00', checkin_cutoff: '07:30', grace_minutes: 0, close_time: '12:00', is_active: true }, item ?? {})
  form.weekdays = [...form.weekdays]; form.checkin_start = form.checkin_start.slice(0, 5); form.checkin_cutoff = form.checkin_cutoff.slice(0, 5); form.close_time = form.close_time.slice(0, 5)
  try { if (!item) classes.value = await options<SchoolClass>('/classes', { active: 'true' }); open.value = true }
  catch (error) { message.value = apiMessage(error) }
}
async function save() {
  saving.value = true; message.value = ''
  try {
    const body = { name: form.name, weekdays: form.weekdays, checkin_start: form.checkin_start, checkin_cutoff: form.checkin_cutoff, grace_minutes: Number(form.grace_minutes), close_time: form.close_time, is_active: form.is_active, ...(!editing.value ? { class_id: form.class_id } : {}) }
    await request(`/attendance-schedules${editing.value ? `/${editing.value}` : ''}`, { method: editing.value ? 'PATCH' : 'POST', body }); open.value = false; success.value = 'Jadwal tersimpan. Sesi yang sudah dibuat tetap memakai jadwal sebelumnya.'; await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function loadCalendar() {
  loading.value = true; message.value = ''
  try { const data = (await request<Result<CalendarDay>>(`/school/calendar/${date.value}`)).data; calendar.is_school_day = data.is_school_day ?? true; calendar.reason = data.is_override ? data.reason : ''; calendarInfo.value = data.is_override ? 'Tanggal ini memiliki pengaturan khusus.' : 'Tanggal ini mengikuti hari aktif pada jadwal kelas.' }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function saveCalendar() {
  saving.value = true; message.value = ''
  try { await request(`/school/calendar/${date.value}`, { method: 'PUT', body: calendar }); success.value = 'Kalender tersimpan. Perubahan berlaku untuk sesi yang dibuat berikutnya. Batalkan sesi terjadwal yang sudah tersedia bila hari menjadi libur.'; await loadCalendar() }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function changePage(value: number) { page.value = value; await load() }
await load()
await loadCalendar()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6"><div><h1 class="text-2xl font-semibold tracking-tight">Jadwal & kalender</h1><p class="mt-2 text-sm text-muted-foreground">Jam lokal sekolah, hari efektif, dan batas pencatatan kehadiran.</p></div><Alert v-if="message && !open" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Tabs v-model="tab" class="gap-4"><TabsList><TabsTrigger value="schedules">Jadwal kelas</TabsTrigger><TabsTrigger value="calendar">Kalender sekolah</TabsTrigger></TabsList>
      <TabsContent value="schedules"><Card class="min-w-0"><CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle>Jadwal absensi</CardTitle><CardDescription class="mt-1.5">Satu jadwal aktif per kelas. Sesi hari ini dibuat otomatis setiap menit.</CardDescription></div><Button v-if="canConfigure" @click="show()"><Plus />Tambah jadwal</Button></CardHeader><CardContent class="min-w-0"><div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat jadwal</span><Skeleton v-for="n in 4" :key="n" class="h-10" /></div><Table v-else class="min-w-[760px]"><TableCaption class="sr-only">Jadwal absensi kelas</TableCaption><TableHeader class="bg-muted/50"><TableRow><TableHead>Jadwal / kelas</TableHead><TableHead>Hari</TableHead><TableHead>Jam / toleransi</TableHead><TableHead>Status</TableHead><TableHead v-if="canConfigure" class="w-12 text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in schedules" :key="item.id"><TableCell class="font-medium">{{ item.name }}<div class="text-xs font-normal text-muted-foreground">{{ item.class_name }} · {{ item.academic_year_name }}</div></TableCell><TableCell class="max-w-48 whitespace-normal">{{ item.weekdays.map(day => dayLabels[day - 1]).join(', ') }}</TableCell><TableCell>{{ item.checkin_start.slice(0, 5) }} – {{ item.close_time.slice(0, 5) }}<div class="text-xs text-muted-foreground">Batas {{ item.checkin_cutoff.slice(0, 5) }} + {{ item.grace_minutes }} menit · {{ item.timezone }}</div></TableCell><TableCell><Badge :variant="statusVariant(item.is_active ? 'active' : 'inactive')">{{ item.is_active ? 'Aktif' : 'Nonaktif' }}</Badge></TableCell><TableCell v-if="canConfigure" class="text-right"><TableActions :label="item.name" :disabled="saving"><DropdownMenuItem @select="show(item)">Edit jadwal</DropdownMenuItem></TableActions></TableCell></TableRow><TableRow v-if="!schedules.length"><TableCell :colspan="canConfigure ? 5 : 4"><Empty><EmptyHeader><EmptyTitle>Belum ada jadwal kelas</EmptyTitle><EmptyDescription>Tambahkan jadwal untuk memulai sesi absensi.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody></Table></CardContent><CardFooter class="block border-t"><TablePagination :page="page" :total="total" label="jadwal" :disabled="loading" @update:page="changePage" /></CardFooter></Card></TabsContent>
      <TabsContent value="calendar"><Card class="max-w-2xl"><CardHeader><CardTitle>Pengaturan hari sekolah</CardTitle><CardDescription>Override hari pada jadwal untuk tanggal tertentu. {{ calendarInfo }}</CardDescription></CardHeader><CardContent><form class="space-y-6" @submit.prevent="saveCalendar"><FieldGroup><Field><FieldLabel for="calendar-date">Tanggal</FieldLabel><Input id="calendar-date" v-model="date" type="date" required :disabled="saving || loading" @change="loadCalendar" /></Field><Field orientation="horizontal"><Switch id="school-day" v-model="calendar.is_school_day" :disabled="!canConfigure || saving || loading" /><FieldContent><FieldLabel for="school-day">Hari efektif sekolah</FieldLabel><FieldDescription>Nonaktifkan untuk hari libur. Batalkan sesi terjadwal yang sudah tersedia. Sesi terbuka tetap perlu ditutup dan ditinjau.</FieldDescription></FieldContent></Field><Field><FieldLabel for="calendar-reason">Alasan pengaturan</FieldLabel><Textarea id="calendar-reason" v-model="calendar.reason" required maxlength="500" :disabled="!canConfigure || saving || loading" placeholder="Contoh: libur nasional atau pengganti hari belajar" /></Field></FieldGroup><Button v-if="canConfigure" type="submit" :disabled="saving || loading"><Spinner v-if="saving" />Simpan kalender</Button></form></CardContent></Card></TabsContent>
    </Tabs>
    <Dialog :open="open" @update:open="value => { if (!saving) open = value }"><DialogContent :show-close-button="!saving" class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"><DialogHeader><DialogTitle>{{ editing ? 'Edit' : 'Tambah' }} jadwal</DialogTitle><DialogDescription>Semua jam memakai zona waktu sekolah. Jadwal berlangsung dalam satu hari.</DialogDescription></DialogHeader><form class="flex flex-col gap-6" @submit.prevent="save"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="schedule-name">Nama jadwal</FieldLabel><Input id="schedule-name" v-model="form.name" required maxlength="120" :disabled="saving" /></Field><Field v-if="!editing"><FieldLabel for="schedule-class">Kelas</FieldLabel><Select v-model="form.class_id" :disabled="saving"><SelectTrigger id="schedule-class" class="w-full"><SelectValue placeholder="Pilih kelas aktif" /></SelectTrigger><SelectContent><SelectItem v-for="item in classes" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select></Field><FieldSet><FieldLegend variant="label">Hari aktif</FieldLegend><FieldGroup class="grid grid-cols-2 gap-3"><Field v-for="(label, index) in dayLabels" :key="label" orientation="horizontal"><Checkbox :id="`weekday-${index}`" :disabled="saving" :model-value="form.weekdays.includes(index + 1)" @update:model-value="value => form.weekdays = value === true ? [...form.weekdays, index + 1] : form.weekdays.filter(day => day !== index + 1)" /><FieldLabel :for="`weekday-${index}`" class="font-normal">{{ label }}</FieldLabel></Field></FieldGroup></FieldSet><div class="grid grid-cols-2 gap-4"><Field><FieldLabel for="schedule-start">Mulai</FieldLabel><Input id="schedule-start" v-model="form.checkin_start" type="time" required :disabled="saving" /></Field><Field><FieldLabel for="schedule-cutoff">Batas tepat waktu</FieldLabel><Input id="schedule-cutoff" v-model="form.checkin_cutoff" type="time" required :disabled="saving" /></Field><Field><FieldLabel for="schedule-close">Tutup</FieldLabel><Input id="schedule-close" v-model="form.close_time" type="time" required :disabled="saving" /></Field><Field><FieldLabel for="schedule-grace">Toleransi (menit)</FieldLabel><Input id="schedule-grace" v-model="form.grace_minutes" type="number" min="0" max="120" required :disabled="saving" /></Field></div><Field orientation="horizontal"><Switch id="schedule-active" v-model="form.is_active" :disabled="saving" /><FieldLabel for="schedule-active">Jadwal aktif</FieldLabel></Field></FieldGroup><DialogFooter><Button type="button" variant="outline" :disabled="saving" @click="open = false">Batal</Button><Button type="submit" :disabled="saving || !form.class_id || !form.weekdays.length"><Spinner v-if="saving" />Simpan jadwal</Button></DialogFooter></form></DialogContent></Dialog>
  </div>
</template>
