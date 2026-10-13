<script setup lang="ts">
import { importSchema, importCommitSchema } from '~/lib/validation/master'
import type { Result } from '~/types/auth'
import type { SchoolClass } from '~/types/master'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canWrite, dateTime } = useMasterData()
interface Preview {
  id: string
  hash: string
  expires_at: string
  valid: boolean
  row_count: number
  errors: { row: number; message: string }[]
  preview: { nis: string; nisn: string; full_name: string }[]
}
const classes = ref<SchoolClass[]>([])
const classId = ref('')
const csv = ref('')
const preview = ref<Preview | null>(null)
const busy = ref(false)
const error = ref('')
const success = ref('')
const filename = ref('')
const { errors, validate, clear } = useFormValidation(
  () => importSchema,
  () => ({ class_id: classId.value, csv: csv.value }),
)
const {
  errors: commitErrors,
  validate: validateCommit,
  clear: clearCommit,
} = useFormValidation(
  () => importCommitSchema,
  () => ({ id: preview.value?.id ?? '', hash: preview.value?.hash ?? '' }),
)
async function file(event: Event) {
  clear()
  clearCommit()
  preview.value = null
  csv.value = ''
  filename.value = ''
  error.value = ''
  success.value = ''
  const input = event.target as HTMLInputElement
  const selected = input.files?.[0]
  if (!selected) return
  if (selected.size > 120 * 1024) {
    error.value = 'File maksimal 120 KiB dan 500 siswa.'
    input.value = ''
    return
  }
  try {
    csv.value = await selected.text()
    filename.value = selected.name
  } catch {
    error.value = 'File tidak dapat dibaca. Pilih ulang file CSV.'
    input.value = ''
  }
}
async function dryRun() {
  if (busy.value || !validate()) return
  busy.value = true
  error.value = ''
  success.value = ''
  preview.value = null
  try {
    preview.value = (
      await request<Result<Preview>>('/students/imports/dry-run', {
        method: 'POST',
        body: { class_id: classId.value, csv: csv.value },
      })
    ).data
  } catch (e) {
    error.value = apiMessage(e)
  } finally {
    busy.value = false
  }
}
async function commit() {
  if (!preview.value?.valid || busy.value || !validateCommit()) return
  busy.value = true
  error.value = ''
  try {
    const result = await request<Result<{ imported: number }>>(
      `/students/imports/${preview.value.id}/commit`,
      { method: 'POST', body: { hash: preview.value.hash } },
    )
    success.value = `${result.data.imported} siswa berhasil diimport. Retry batch ini tidak membuat duplikat.`
    clear()
    clearCommit()
    preview.value = null
    csv.value = ''
    filename.value = ''
  } catch (e) {
    error.value = apiMessage(e)
  } finally {
    busy.value = false
  }
}
if (canWrite.value) {
  try {
    classes.value = await options<SchoolClass>('/classes', { active: 'true' })
  } catch (e) {
    error.value = apiMessage(e)
  }
}
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div>
      <Button
        as-child
        variant="ghost"
        size="sm"
        class="-ml-3"
      >
        <NuxtLink to="/admin/students">Kembali ke siswa</NuxtLink>
      </Button>
      <PageHeader
        title="Import siswa"
        description="Tinjau seluruh baris sebelum menyimpan siswa dan penempatan kelas."
        class="mt-4"
      >
        <template #actions>
          <Button
            v-if="canWrite"
            type="submit"
            form="import-preview"
            :disabled="busy"
          >
            <Spinner v-if="busy" />
            Tinjau import
          </Button>
        </template>
      </PageHeader>
    </div>
    <Alert
      v-if="!canWrite"
      variant="destructive"
    >
      <AlertDescription>Anda tidak memiliki izin import siswa.</AlertDescription>
    </Alert>
    <template v-else>
      <Alert
        v-if="error"
        variant="destructive"
      >
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <Alert
        v-if="success"
        role="status"
      >
        <AlertDescription>{{ success }}</AlertDescription>
      </Alert>
      <Card>
        <CardHeader>
          <CardTitle>File CSV & kelas tujuan</CardTitle>
          <CardDescription>
            Header wajib: nis,nisn,full_name. Satu file untuk satu kelas, maksimal 500 siswa. NIS
            existing ditolak. Preview berlaku 15 menit.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            id="import-preview"
            novalidate
            class="space-y-6"
            @submit.prevent="dryRun"
          >
            <FieldGroup class="sm:grid sm:grid-cols-2">
              <Field :data-invalid="!!errors.class_id">
                <FieldLabel for="import-class">Kelas tujuan</FieldLabel>
                <Select
                  v-model="classId"
                  :disabled="busy"
                  @update:model-value="preview = null"
                >
                  <SelectTrigger
                    id="import-class"
                    :aria-invalid="!!errors.class_id"
                    :aria-describedby="errors.class_id ? 'import-class-error' : undefined"
                    class="w-full"
                  >
                    <SelectValue placeholder="Pilih kelas aktif" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="item in classes"
                      :key="item.id"
                      :value="item.id"
                    >
                      {{ item.name }} · {{ item.academic_year_name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FieldError
                  v-if="errors.class_id"
                  id="import-class-error"
                >
                  {{ errors.class_id }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.csv">
                <FieldLabel for="import-file">File CSV</FieldLabel>
                <Input
                  id="import-file"
                  :aria-invalid="!!errors.csv"
                  :aria-describedby="errors.csv ? 'import-file-error' : undefined"
                  type="file"
                  accept=".csv,text/csv"
                  :disabled="busy"
                  @change="file"
                />
                <FieldDescription>{{ filename || 'UTF-8, maksimal 120 KiB.' }}</FieldDescription>
                <FieldError
                  v-if="errors.csv"
                  id="import-file-error"
                >
                  {{ errors.csv }}
                </FieldError>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <Card
        v-if="preview"
        class="min-w-0"
      >
        <CardHeader>
          <CardTitle>Preview {{ preview.row_count }} siswa</CardTitle>
          <CardDescription>
            Berlaku sampai {{ dateTime(preview.expires_at) }}. Semua baris disimpan dalam satu
            transaksi atau seluruhnya dibatalkan.
          </CardDescription>
          <CardAction>
            <Button
              :disabled="busy || !preview.valid"
              @click="commit"
            >
              <Spinner v-if="busy" />
              Simpan {{ preview.row_count }} siswa
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent class="space-y-4">
          <FieldError v-if="commitErrors.id || commitErrors.hash">
            {{ commitErrors.id || commitErrors.hash }}
          </FieldError>
          <Alert
            v-if="!preview.valid"
            variant="destructive"
          >
            <AlertTitle>Perbaiki file sebelum import</AlertTitle>
            <AlertDescription>
              <ul class="list-inside list-disc">
                <li
                  v-for="(issue, i) in preview.errors"
                  :key="i"
                >
                  Baris {{ issue.row }}: {{ issue.message }}
                </li>
              </ul>
            </AlertDescription>
          </Alert>
          <Table>
            <TableCaption class="sr-only">Preview data siswa yang akan diimport</TableCaption>
            <TableHeader class="bg-muted/50">
              <TableRow>
                <TableHead>NIS</TableHead>
                <TableHead>NISN</TableHead>
                <TableHead>Nama</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow
                v-for="(row, i) in preview.preview"
                :key="i"
              >
                <TableCell>{{ row.nis }}</TableCell>
                <TableCell>{{ row.nisn || '—' }}</TableCell>
                <TableCell>{{ row.full_name }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </template>
  </div>
</template>
