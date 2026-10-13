<script setup lang="ts">
import type { Result } from '~/types/auth'
import type { SchoolClass } from '~/types/master'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canWrite, dateTime } = useMasterData()
interface Preview { id: string, hash: string, expires_at: string, valid: boolean, row_count: number, errors: { row: number, message: string }[], preview: { nis: string, nisn: string, full_name: string }[] }
const classes = ref<SchoolClass[]>([]); const classId = ref(''); const csv = ref('')
const preview = ref<Preview | null>(null); const busy = ref(false); const error = ref(''); const success = ref('')
const filename = ref('')
async function file(event: Event) {
  preview.value = null; csv.value = ''; error.value = ''; success.value = ''
  const input = event.target as HTMLInputElement; const selected = input.files?.[0]
  if (!selected) return
  if (selected.size > 120 * 1024) { error.value = 'File maksimal 120 KiB dan 500 siswa.'; input.value = ''; return }
  csv.value = await selected.text(); filename.value = selected.name
}
async function dryRun() {
  busy.value = true; error.value = ''; success.value = ''; preview.value = null
  try { preview.value = (await request<Result<Preview>>('/students/imports/dry-run', { method: 'POST', body: { class_id: classId.value, csv: csv.value } })).data }
  catch (e) { error.value = apiMessage(e) }
  finally { busy.value = false }
}
async function commit() {
  if (!preview.value?.valid) return
  busy.value = true; error.value = ''
  try { const result = await request<Result<{ imported: number }>>(`/students/imports/${preview.value.id}/commit`, { method: 'POST', body: { hash: preview.value.hash } }); success.value = `${result.data.imported} siswa berhasil diimport. Retry batch ini tidak membuat duplikat.`; preview.value = null; csv.value = ''; filename.value = '' }
  catch (e) { error.value = apiMessage(e) }
  finally { busy.value = false }
}
if (canWrite.value) { try { classes.value = await options<SchoolClass>('/classes', { active: 'true' }) } catch (e) { error.value = apiMessage(e) } }
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6"><div><Button as-child variant="ghost" size="sm" class="-ml-3"><NuxtLink to="/admin/students">Kembali ke siswa</NuxtLink></Button><h1 class="mt-4 text-2xl font-semibold tracking-tight">Import siswa</h1><p class="mt-2 text-sm text-muted-foreground">Tinjau seluruh baris sebelum menyimpan siswa dan penempatan kelas.</p></div><Alert v-if="!canWrite" variant="destructive"><AlertDescription>Anda tidak memiliki izin import siswa.</AlertDescription></Alert><template v-else><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Card><CardHeader><CardTitle>File CSV & kelas tujuan</CardTitle><CardDescription>Header wajib: nis,nisn,full_name. Satu file untuk satu kelas, maksimal 500 siswa. NIS existing ditolak. Preview berlaku 15 menit.</CardDescription></CardHeader><CardContent><form class="space-y-6" @submit.prevent="dryRun"><FieldGroup class="sm:grid sm:grid-cols-2"><Field><FieldLabel for="import-class">Kelas tujuan</FieldLabel><Select v-model="classId" :disabled="busy" @update:model-value="preview = null"><SelectTrigger id="import-class" class="w-full"><SelectValue placeholder="Pilih kelas aktif" /></SelectTrigger><SelectContent><SelectItem v-for="item in classes" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="import-file">File CSV</FieldLabel><Input id="import-file" type="file" accept=".csv,text/csv" :disabled="busy" @change="file" /><FieldDescription>{{ filename || 'UTF-8, maksimal 120 KiB.' }}</FieldDescription></Field></FieldGroup><Button type="submit" :disabled="busy || !classId || !csv"><Spinner v-if="busy" />Tinjau import</Button></form></CardContent></Card>
    <Card v-if="preview" class="min-w-0"><CardHeader><CardTitle>Preview {{ preview.row_count }} siswa</CardTitle><CardDescription>Berlaku sampai {{ dateTime(preview.expires_at) }}. Semua baris disimpan dalam satu transaksi atau seluruhnya dibatalkan.</CardDescription></CardHeader><CardContent class="space-y-4"><Alert v-if="!preview.valid" variant="destructive"><AlertTitle>Perbaiki file sebelum import</AlertTitle><AlertDescription><ul class="list-inside list-disc"><li v-for="(issue, i) in preview.errors" :key="i">Baris {{ issue.row }}: {{ issue.message }}</li></ul></AlertDescription></Alert><Table><TableCaption class="sr-only">Preview data siswa yang akan diimport</TableCaption><TableHeader><TableRow><TableHead>NIS</TableHead><TableHead>NISN</TableHead><TableHead>Nama</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="(row, i) in preview.preview" :key="i"><TableCell>{{ row.nis }}</TableCell><TableCell>{{ row.nisn || '—' }}</TableCell><TableCell>{{ row.full_name }}</TableCell></TableRow></TableBody></Table><Button :disabled="busy || !preview.valid" @click="commit"><Spinner v-if="busy" />Simpan {{ preview.row_count }} siswa</Button></CardContent></Card>
  </template></div>
</template>
