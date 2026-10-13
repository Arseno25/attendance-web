<script setup lang="ts">
import { Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { AcademicYear, Assignment, SchoolClass, TeacherOption } from '~/types/master'

definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, canWrite, options, dateTime } = useMasterData()
const tab = ref('years')
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const success = ref('')
const open = ref(false)
const editing = ref('')
const years = ref<AcademicYear[]>([])
const classes = ref<SchoolClass[]>([])
const assignments = ref<Assignment[]>([])
const yearOptions = ref<AcademicYear[]>([])
const classOptions = ref<SchoolClass[]>([])
const teachers = ref<TeacherOption[]>([])
const revoke = ref<Assignment | null>(null)
const form = reactive({ name: '', starts_on: '', ends_on: '', is_active: false, code: '', level: '', academic_year_id: '', class_id: '', teacher_id: '' })
const labels: Record<string, string> = { years: 'tahun ajaran', classes: 'kelas', assignments: 'penugasan' }
const paths: Record<string, string> = { years: '/academic-years', classes: '/classes', assignments: '/teacher-assignments' }
let loadSequence = 0
async function load() {
  const sequence = ++loadSequence
  const kind = tab.value
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<AcademicYear[] | SchoolClass[] | Assignment[]>>(paths[kind]!, { query: { page: page.value, page_size: 20 } })
    if (sequence !== loadSequence) return
    if (kind === 'years') years.value = result.data as AcademicYear[]
    else if (kind === 'classes') classes.value = result.data as SchoolClass[]
    else assignments.value = result.data as Assignment[]
    total.value = result.pagination?.total ?? 0
  }
  catch (error) { if (sequence === loadSequence) message.value = apiMessage(error) }
  finally { if (sequence === loadSequence) loading.value = false }
}
watch(tab, async () => { page.value = 1; await load() })
async function show(item?: AcademicYear | SchoolClass) {
  message.value = ''; success.value = ''; editing.value = item?.id ?? ''
  Object.assign(form, { name: '', starts_on: '', ends_on: '', is_active: tab.value === 'classes', code: '', level: '', academic_year_id: '', class_id: '', teacher_id: '' }, item ?? {})
  try {
    if (tab.value === 'classes') yearOptions.value = await options<AcademicYear>('/academic-years')
    if (tab.value === 'assignments') {
      [classOptions.value, teachers.value] = await Promise.all([options<SchoolClass>('/classes', { active: 'true' }), options<TeacherOption>('/teacher-options')])
    }
    open.value = true
  }
  catch (error) { message.value = apiMessage(error) }
}
async function save() {
  saving.value = true; message.value = ''
  try {
    const body = tab.value === 'years' ? { name: form.name, starts_on: form.starts_on, ends_on: form.ends_on, is_active: form.is_active }
      : tab.value === 'classes' ? { name: form.name, code: form.code, level: form.level, is_active: form.is_active, ...(!editing.value ? { academic_year_id: form.academic_year_id } : {}) }
        : { teacher_id: form.teacher_id, class_id: form.class_id }
    await request(`${paths[tab.value]}${editing.value ? `/${editing.value}` : ''}`, { method: editing.value ? 'PATCH' : 'POST', body })
    open.value = false; success.value = 'Data berhasil disimpan.'; await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function revokeAssignment() {
  if (!revoke.value) return
  saving.value = true; message.value = ''
  try {
    await request(`/teacher-assignments/${revoke.value.id}`, { method: 'PATCH', body: { is_active: false } })
    revoke.value = null; success.value = 'Penugasan dicabut. Akses kelas guru diperbarui segera.'; await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function changePage(value: number) { page.value = value; await load() }
await load()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Data akademik</h1><p class="mt-2 text-sm text-muted-foreground">{{ canWrite ? 'Kelola tahun ajaran, kelas, dan penugasan guru.' : 'Kelas dan penugasan yang dapat Anda akses.' }}</p></div>
    <Alert v-if="message && !open && !revoke" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Tabs v-model="tab" class="min-w-0 gap-4">
      <TabsList class="w-full sm:w-auto"><TabsTrigger value="years" :disabled="loading">Tahun ajaran</TabsTrigger><TabsTrigger value="classes" :disabled="loading">Kelas</TabsTrigger><TabsTrigger value="assignments" :disabled="loading">Guru</TabsTrigger></TabsList>
      <Card class="min-w-0">
        <CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4"><div><CardTitle class="capitalize">{{ labels[tab] }}</CardTitle><CardDescription class="mt-1.5">{{ total }} {{ labels[tab] }} tercatat.</CardDescription></div><Button v-if="canWrite" @click="show()"><Plus />Tambah {{ labels[tab] }}</Button></CardHeader>
        <CardContent class="min-w-0">
          <div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat data akademik</span><Skeleton v-for="n in 4" :key="n" class="h-10" /></div>
          <TabsContent v-for="kind in ['years', 'classes', 'assignments']" v-else :key="kind" :value="kind" class="mt-0">
            <Table class="min-w-[620px]">
              <TableCaption class="sr-only">Daftar {{ labels[kind] }}</TableCaption>
              <TableHeader><TableRow><TableHead>{{ kind === 'years' ? 'Tahun ajaran' : kind === 'classes' ? 'Kelas' : 'Guru' }}</TableHead><TableHead>{{ kind === 'years' ? 'Periode' : kind === 'classes' ? 'Tahun ajaran' : 'Kelas / tahun ajaran' }}</TableHead><TableHead>Status</TableHead><TableHead v-if="canWrite" class="text-right">Tindakan</TableHead></TableRow></TableHeader>
              <TableBody v-if="kind === 'years'"><TableRow v-for="year in years" :key="year.id"><TableCell class="font-medium">{{ year.name }}</TableCell><TableCell>{{ year.starts_on }} – {{ year.ends_on }}</TableCell><TableCell><Badge :variant="year.is_active ? 'secondary' : 'outline'">{{ year.is_active ? 'Aktif' : 'Nonaktif' }}</Badge></TableCell><TableCell v-if="canWrite" class="text-right"><Button variant="ghost" size="sm" @click="show(year)">Edit</Button></TableCell></TableRow></TableBody>
              <TableBody v-else-if="kind === 'classes'"><TableRow v-for="item in classes" :key="item.id"><TableCell><div class="font-medium">{{ item.name }}</div><div class="text-xs text-muted-foreground">{{ item.code }} · {{ item.level || 'Tingkat belum diisi' }}</div></TableCell><TableCell>{{ item.academic_year_name }}</TableCell><TableCell><Badge :variant="item.is_active ? 'secondary' : 'outline'">{{ item.is_active ? 'Aktif' : 'Nonaktif' }}</Badge></TableCell><TableCell v-if="canWrite" class="text-right"><Button variant="ghost" size="sm" @click="show(item)">Edit</Button></TableCell></TableRow></TableBody>
              <TableBody v-else><TableRow v-for="item in assignments" :key="item.id"><TableCell><div class="font-medium">{{ item.teacher_name }}</div><div class="text-xs text-muted-foreground">{{ dateTime(item.starts_at) }} – {{ dateTime(item.ends_at) }}</div></TableCell><TableCell>{{ item.class_name }}<div class="text-xs text-muted-foreground">{{ item.academic_year_name }}</div></TableCell><TableCell><Badge :variant="item.is_current ? 'secondary' : 'outline'">{{ !item.is_active ? 'Dicabut' : item.is_current ? 'Aktif' : 'Di luar periode' }}</Badge></TableCell><TableCell v-if="canWrite" class="text-right"><Button v-if="item.is_active" variant="ghost" size="sm" @click="message = ''; revoke = item">Cabut</Button><span v-else class="text-muted-foreground">—</span></TableCell></TableRow></TableBody>
            </Table>
            <Empty v-if="!total" class="py-12"><EmptyHeader><EmptyTitle>Belum ada {{ labels[kind] }}</EmptyTitle><EmptyDescription>{{ canWrite ? 'Tambahkan data untuk memulai pengelolaan sekolah.' : 'Minta admin sekolah mengatur penugasan kelas Anda.' }}</EmptyDescription></EmptyHeader></Empty>
          </TabsContent>
        </CardContent>
        <CardFooter class="block border-t"><TablePagination :page="page" :total="total" :label="labels[tab] ?? 'data'" :disabled="loading" @update:page="changePage" /></CardFooter>
      </Card>
    </Tabs>
    <Sheet :open="open" @update:open="value => { if (!saving) open = value }"><SheetContent class="overflow-y-auto sm:max-w-lg"><SheetHeader><SheetTitle>{{ editing ? 'Edit' : 'Tambah' }} {{ labels[tab] }}</SheetTitle><SheetDescription>{{ tab === 'years' ? 'Tanggal dikunci setelah kelas dibuat. Aktivasi menggantikan tahun yang sebelumnya aktif.' : tab === 'classes' ? 'Kelas selalu terikat pada satu tahun ajaran.' : 'Guru mendapat akses siswa pada kelas ini selama tahun ajaran aktif.' }}</SheetDescription></SheetHeader>
      <form class="space-y-6 px-4 pb-6" @submit.prevent="save"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup>
        <template v-if="tab === 'years'"><Field><FieldLabel for="year-name">Nama tahun ajaran</FieldLabel><Input id="year-name" v-model="form.name" required maxlength="120" :disabled="saving" placeholder="2026/2027" /></Field><Field><FieldLabel for="year-start">Tanggal mulai</FieldLabel><Input id="year-start" v-model="form.starts_on" type="date" required :disabled="saving" /></Field><Field><FieldLabel for="year-end">Tanggal akhir</FieldLabel><Input id="year-end" v-model="form.ends_on" type="date" required :min="form.starts_on" :disabled="saving" /></Field></template>
        <template v-else-if="tab === 'classes'"><Field v-if="!editing"><FieldLabel for="class-year">Tahun ajaran</FieldLabel><Select v-model="form.academic_year_id" :disabled="saving"><SelectTrigger id="class-year" class="w-full"><SelectValue placeholder="Pilih tahun ajaran" /></SelectTrigger><SelectContent><SelectItem v-for="year in yearOptions" :key="year.id" :value="year.id">{{ year.name }}</SelectItem></SelectContent></Select><FieldDescription v-if="!yearOptions.length">Tambahkan tahun ajaran terlebih dahulu.</FieldDescription></Field><Field><FieldLabel for="class-code">Kode kelas</FieldLabel><Input id="class-code" v-model="form.code" required maxlength="30" :disabled="saving" placeholder="7A" /></Field><Field><FieldLabel for="class-name">Nama kelas</FieldLabel><Input id="class-name" v-model="form.name" required maxlength="120" :disabled="saving" placeholder="Kelas 7 A" /></Field><Field><FieldLabel for="class-level">Tingkat (opsional)</FieldLabel><Input id="class-level" v-model="form.level" maxlength="30" :disabled="saving" placeholder="7" /></Field></template>
        <template v-else><Field><FieldLabel for="assignment-teacher">Guru</FieldLabel><Select v-model="form.teacher_id" :disabled="saving"><SelectTrigger id="assignment-teacher" class="w-full"><SelectValue placeholder="Pilih guru aktif" /></SelectTrigger><SelectContent><SelectItem v-for="teacher in teachers" :key="teacher.id" :value="teacher.id">{{ teacher.name }}</SelectItem></SelectContent></Select><FieldDescription v-if="!teachers.length">Buat akun guru aktif melalui Pengguna & peran terlebih dahulu.</FieldDescription></Field><Field><FieldLabel for="assignment-class">Kelas</FieldLabel><Select v-model="form.class_id" :disabled="saving"><SelectTrigger id="assignment-class" class="w-full"><SelectValue placeholder="Pilih kelas aktif" /></SelectTrigger><SelectContent><SelectItem v-for="item in classOptions" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select><FieldDescription>Penugasan mulai sekarang sampai akhir tahun ajaran.</FieldDescription></Field></template>
        <Field v-if="tab !== 'assignments'" orientation="horizontal"><Switch id="master-active" v-model="form.is_active" :disabled="saving" /><FieldLabel for="master-active">{{ tab === 'years' ? 'Jadikan tahun ajaran aktif' : 'Kelas aktif' }}</FieldLabel></Field>
      </FieldGroup><Button type="submit" class="w-full" :disabled="saving || (tab === 'classes' && !editing && !form.academic_year_id) || (tab === 'assignments' && (!form.class_id || !form.teacher_id))"><Spinner v-if="saving" />Simpan {{ labels[tab] }}</Button></form>
    </SheetContent></Sheet>
    <Dialog :open="!!revoke" @update:open="value => { if (!value && !saving) revoke = null }"><DialogContent><DialogHeader><DialogTitle>Cabut penugasan guru?</DialogTitle><DialogDescription>{{ revoke?.teacher_name }} akan kehilangan akses ke {{ revoke?.class_name }} dari penugasan ini. Histori penugasan tetap tersimpan.</DialogDescription></DialogHeader><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><DialogFooter><Button variant="outline" :disabled="saving" @click="revoke = null">Batal</Button><Button variant="destructive" :disabled="saving" @click="revokeAssignment"><Spinner v-if="saving" />Cabut penugasan</Button></DialogFooter></DialogContent></Dialog>
  </div>
</template>
