<script setup lang="ts">
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
async function load() {
  loading.value = true; message.value = ''
  try { const result = await request<Result<AttendanceSession[]>>('/attendance-sessions', { query: { date: date.value, page: page.value, page_size: 20 } }); sessions.value = result.data; total.value = result.pagination?.total ?? 0 }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function show() {
  message.value = ''; form.schedule_id = ''; form.date = date.value || today()
  try { schedules.value = (await options<Schedule>('/attendance-schedules')).filter(item => item.is_active); open.value = true }
  catch (error) { message.value = apiMessage(error) }
}
async function save() {
  saving.value = true; message.value = ''
  try { const data = (await request<Result<AttendanceSession>>('/attendance-sessions', { method: 'POST', body: form })).data; open.value = false; await navigateTo(`/admin/attendance/${data.id}`) }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function filter() { page.value = 1; await load() }
async function changePage(value: number) { page.value = value; await load() }
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6"><div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">Sesi absensi</h1><p class="mt-2 text-sm text-muted-foreground">Catat kehadiran, tinjau yang belum tercatat, lalu rekonsiliasi setelah sesi ditutup.</p></div><Button v-if="canManual" @click="show"><Plus />Buat sesi</Button></div><Alert v-if="message && !open" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <Card class="min-w-0"><CardHeader class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div><CardTitle>Sesi sekolah</CardTitle><CardDescription class="mt-1.5">Sesi hari ini dibuat otomatis dari jadwal aktif. Semua tanggal mengikuti zona waktu sekolah.</CardDescription></div><form class="flex gap-2" @submit.prevent="filter"><Label for="session-date" class="sr-only">Tanggal sesi</Label><Input id="session-date" v-model="date" type="date" :disabled="loading" /><Button variant="outline" type="submit" :disabled="loading">Lihat</Button></form></CardHeader><CardContent class="min-w-0"><div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat sesi</span><Skeleton v-for="n in 4" :key="n" class="h-10" /></div><Table v-else class="min-w-[720px]"><TableCaption class="sr-only">Daftar sesi absensi sekolah</TableCaption><TableHeader><TableRow><TableHead>Kelas / jadwal</TableHead><TableHead>Periode</TableHead><TableHead>Status</TableHead><TableHead>Tercatat / roster</TableHead><TableHead class="text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in sessions" :key="item.id"><TableCell class="font-medium">{{ item.class_name }}<div class="text-xs font-normal text-muted-foreground">{{ item.name }}</div></TableCell><TableCell>{{ dateTime(item.starts_at) }}<div class="text-xs text-muted-foreground">Sampai {{ dateTime(item.closes_at) }}</div></TableCell><TableCell><Badge :variant="item.status === 'open' ? 'secondary' : 'outline'">{{ sessionLabels[item.status] }}</Badge><div v-if="item.reconciled_at" class="mt-1 text-xs text-muted-foreground">Sudah direkonsiliasi</div></TableCell><TableCell class="tabular-nums">{{ item.recorded_count }} / {{ item.roster_count }}</TableCell><TableCell class="text-right"><Button as-child variant="ghost" size="sm"><NuxtLink :to="`/admin/attendance/${item.id}`">Buka detail</NuxtLink></Button></TableCell></TableRow><TableRow v-if="!sessions.length"><TableCell colspan="5"><Empty><EmptyHeader><EmptyTitle>Belum ada sesi pada tanggal ini</EmptyTitle><EmptyDescription>Periksa jadwal kelas dan kalender, atau buat sesi dari jadwal aktif.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow></TableBody></Table></CardContent><CardFooter class="block border-t"><TablePagination :page="page" :total="total" label="sesi" :disabled="loading" @update:page="changePage" /></CardFooter></Card>
    <Sheet :open="open" @update:open="value => { if (!saving) open = value }"><SheetContent class="overflow-y-auto"><SheetHeader><SheetTitle>Buat sesi absensi</SheetTitle><SheetDescription>Sesi mengikuti jadwal dan kalender. Roster dibekukan saat sesi dibuka.</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="save"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="session-schedule">Jadwal</FieldLabel><Select v-model="form.schedule_id" :disabled="saving"><SelectTrigger id="session-schedule" class="w-full"><SelectValue placeholder="Pilih jadwal aktif" /></SelectTrigger><SelectContent><SelectItem v-for="item in schedules" :key="item.id" :value="item.id">{{ item.class_name }} · {{ item.name }}</SelectItem></SelectContent></Select><FieldDescription v-if="!schedules.length">Tambahkan jadwal aktif terlebih dahulu.</FieldDescription></Field><Field><FieldLabel for="new-session-date">Tanggal</FieldLabel><Input id="new-session-date" v-model="form.date" type="date" required :disabled="saving" /></Field></FieldGroup><Button class="w-full" type="submit" :disabled="saving || !form.schedule_id"><Spinner v-if="saving" />Buat sesi</Button></form></SheetContent></Sheet>
  </div>
</template>
