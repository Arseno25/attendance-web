<script setup lang="ts">
import { RefreshCw, ScanFace } from '@lucide/vue'
import { searchSchema } from '~/lib/validation/master'
import { statusVariant } from '~/lib/status'
import { statusLabels, type Student } from '~/types/master'
import type { Result } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'biometrics'] })
const request = useApi()
const students = ref<Pick<Student, 'id' | 'full_name' | 'nis' | 'status' | 'class_name'>[]>([])
const selected = ref<(typeof students.value)[number] | null>(null)
const page = ref(1)
const total = ref(0)
const search = ref('')
const loading = ref(false)
const error = ref('')
const { errors, validate } = useFormValidation(
  () => searchSchema,
  () => ({ search: search.value }),
)

async function load() {
  if (loading.value || !validate()) return
  loading.value = true
  error.value = ''
  try {
    const result = await request<Result<typeof students.value>>('/biometrics/students', {
      query: { page: page.value, page_size: 20, search: search.value.trim() },
    })
    students.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (cause) {
    error.value = apiMessage(cause)
  } finally {
    loading.value = false
  }
}
async function filter() {
  page.value = 1
  await load()
}
async function changePage(value: number) {
  page.value = value
  await load()
}
await load()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Wajah siswa"
      description="Kelola pendaftaran dan pencabutan template sesuai izin petugas."
    >
      <template #actions>
        <Button
          variant="outline"
          :disabled="loading"
          @click="load"
        >
          <RefreshCw :class="loading ? 'animate-spin' : ''" />
          Muat ulang
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="error"
      variant="destructive"
      role="alert"
    >
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Daftar siswa</CardTitle>
        <CardDescription>
          {{ total }} siswa · akurasi dan liveness belum dievaluasi.
        </CardDescription>
      </CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <form
          novalidate
          class="flex flex-wrap items-start gap-3"
          @submit.prevent="filter"
        >
          <Field
            class="min-w-0 flex-1 sm:max-w-sm"
            :data-invalid="!!errors.search"
          >
            <FieldLabel
              for="face-student-search"
              class="sr-only"
            >
              Cari nama atau NIS
            </FieldLabel>
            <Input
              id="face-student-search"
              v-model="search"
              placeholder="Cari nama atau NIS"
              :disabled="loading"
              :aria-invalid="!!errors.search"
              :aria-describedby="errors.search ? 'face-search-error' : undefined"
            />
            <FieldError
              v-if="errors.search"
              id="face-search-error"
            >
              {{ errors.search }}
            </FieldError>
          </Field>
          <Button
            type="submit"
            variant="outline"
            :disabled="loading"
          >
            Cari
          </Button>
        </form>
        <Table>
          <TableCaption class="sr-only">Siswa untuk pengelolaan wajah</TableCaption>
          <TableHeader class="bg-muted/50">
            <TableRow>
              <TableHead>NIS</TableHead>
              <TableHead>Nama</TableHead>
              <TableHead>Kelas</TableHead>
              <TableHead>Status</TableHead>
              <TableHead class="w-12 text-right">Tindakan</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="student in students"
              :key="student.id"
            >
              <TableCell>{{ student.nis }}</TableCell>
              <TableCell class="font-medium">{{ student.full_name }}</TableCell>
              <TableCell>{{ student.class_name ?? 'Belum ditempatkan' }}</TableCell>
              <TableCell>
                <Badge :variant="statusVariant(student.status)">
                  {{ statusLabels[student.status] }}
                </Badge>
              </TableCell>
              <TableCell class="text-right">
                <TableActions
                  :label="student.full_name"
                  :disabled="loading"
                >
                  <DropdownMenuItem @select="selected = student">
                    <ScanFace />
                    Kelola wajah
                  </DropdownMenuItem>
                </TableActions>
              </TableCell>
            </TableRow>
            <TableRow v-if="!students.length">
              <TableCell
                colspan="5"
                class="py-8 text-center text-muted-foreground"
              >
                {{ loading ? 'Memuat siswa…' : 'Tidak ada siswa sesuai pencarian.' }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <TablePagination
          :page="page"
          :total="total"
          label="siswa"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardContent>
    </Card>
    <Dialog
      :open="!!selected"
      @update:open="
        (value) => {
          if (!value) selected = null
        }
      "
    >
      <DialogContent class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{{ selected?.full_name }}</DialogTitle>
          <DialogDescription>
            NIS {{ selected?.nis }} · {{ selected?.class_name ?? 'Belum ditempatkan' }}
          </DialogDescription>
        </DialogHeader>
        <StudentFaceEnrollment
          v-if="selected"
          :student-id="selected.id"
        />
      </DialogContent>
    </Dialog>
  </div>
</template>
