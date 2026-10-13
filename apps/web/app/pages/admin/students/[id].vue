<script setup lang="ts">
import { ArrowLeft, ArrowRightLeft, Pencil } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { statusLabels, type Student, type SchoolClass } from '~/types/master'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canWrite, dateTime } = useMasterData()
const route = useRoute()
const student = ref<Student | null>(null)
const message = ref('')
const success = ref('')
const editOpen = ref(false)
const placementOpen = ref(false)
const saving = ref(false)
const classes = ref<SchoolClass[]>([])
const form = reactive({ class_id: '', reason: '' })
async function load() {
  message.value = ''
  try { student.value = (await request<Result<Student>>(`/students/${route.params.id}`)).data }
  catch (error) { student.value = null; message.value = apiMessage(error) }
}
async function place() {
  message.value = ''; success.value = ''; form.class_id = ''; form.reason = ''
  try { classes.value = await options<SchoolClass>('/classes', { active: 'true' }); placementOpen.value = true }
  catch (error) { message.value = apiMessage(error) }
}
async function savePlacement() {
  saving.value = true; message.value = ''
  try {
    await request(`/students/${route.params.id}/placements`, { method: 'POST', body: form })
    placementOpen.value = false; success.value = 'Penempatan baru tersimpan; histori sebelumnya tetap tersedia.'; await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function saved() { success.value = 'Data siswa berhasil diperbarui.'; await load() }
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div><Button as-child variant="ghost" size="sm" class="-ml-3"><NuxtLink to="/admin/students"><ArrowLeft />Daftar siswa</NuxtLink></Button></div>
    <Alert v-if="message && !placementOpen" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <template v-if="student"><div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">{{ student.full_name }}</h1><p class="mt-2 text-sm text-muted-foreground">NIS {{ student.nis }} · {{ student.class_name ?? 'Belum ditempatkan' }}</p></div><div v-if="canWrite" class="flex flex-wrap gap-2"><Button variant="outline" @click="editOpen = true"><Pencil />Edit siswa</Button><Button v-if="student.status === 'active'" @click="place"><ArrowRightLeft />Penempatan kelas</Button></div></div>
      <Card><CardHeader><CardTitle>Identitas siswa</CardTitle><CardDescription>Informasi siswa di sekolah Anda.</CardDescription></CardHeader><CardContent><dl class="grid gap-6 sm:grid-cols-2 xl:grid-cols-4"><div><dt class="text-sm text-muted-foreground">NIS</dt><dd class="mt-1 font-medium">{{ student.nis }}</dd></div><div><dt class="text-sm text-muted-foreground">NISN</dt><dd class="mt-1 font-medium">{{ student.nisn ?? 'Belum diisi' }}</dd></div><div><dt class="text-sm text-muted-foreground">Status</dt><dd class="mt-1"><Badge :variant="student.status === 'active' ? 'secondary' : 'outline'">{{ statusLabels[student.status] }}</Badge></dd></div><div><dt class="text-sm text-muted-foreground">Tahun ajaran terakhir</dt><dd class="mt-1 font-medium">{{ student.academic_year_name ?? '—' }}</dd></div></dl></CardContent></Card>
      <Card class="min-w-0"><CardHeader><CardTitle>Histori penempatan</CardTitle><CardDescription>Waktu akhir menandai berakhirnya penempatan. {{ canWrite ? 'Histori tidak dihapus saat siswa pindah kelas.' : 'Hanya histori pada kelas yang dapat Anda akses ditampilkan.' }}</CardDescription></CardHeader><CardContent class="min-w-0"><Table class="min-w-[720px]"><TableCaption class="sr-only">Histori kelas siswa</TableCaption><TableHeader><TableRow><TableHead>Kelas</TableHead><TableHead>Mulai</TableHead><TableHead>Berakhir</TableHead><TableHead>Alasan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in student.placements" :key="item.id"><TableCell class="font-medium">{{ item.class_name }}<div class="text-xs font-normal text-muted-foreground">{{ item.academic_year_name }}</div></TableCell><TableCell>{{ dateTime(item.joined_at) }}</TableCell><TableCell>{{ dateTime(item.left_at) }}</TableCell><TableCell class="max-w-64 whitespace-normal">{{ item.reason }}</TableCell></TableRow><TableRow v-if="!student.placements?.length"><TableCell colspan="4" class="py-8 text-center text-muted-foreground">Belum ada penempatan yang dapat ditampilkan.</TableCell></TableRow></TableBody></Table></CardContent></Card>
      <StudentEditor v-if="canWrite" v-model:open="editOpen" :student="student" @saved="saved" />
    </template>
    <Sheet :open="placementOpen" @update:open="value => { if (!saving) placementOpen = value }"><SheetContent class="overflow-y-auto"><SheetHeader><SheetTitle>Penempatan kelas</SheetTitle><SheetDescription>Penempatan dimulai sekarang dan berakhir di akhir tahun ajaran. Periode kelas sebelumnya akan ditutup.</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="savePlacement"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="placement-class">Kelas tujuan</FieldLabel><Select v-model="form.class_id" :disabled="saving"><SelectTrigger id="placement-class" class="w-full"><SelectValue placeholder="Pilih kelas aktif" /></SelectTrigger><SelectContent><SelectItem v-for="item in classes" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="placement-reason">Alasan penempatan</FieldLabel><Textarea id="placement-reason" v-model="form.reason" required maxlength="500" :disabled="saving" placeholder="Contoh: perpindahan rombongan belajar" /></Field></FieldGroup><Button class="w-full" type="submit" :disabled="saving || !form.class_id"><Spinner v-if="saving" />Simpan penempatan</Button></form></SheetContent></Sheet>
  </div>
</template>
