<script setup lang="ts">
import { RefreshCw } from '@lucide/vue'
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
      description="Cari siswa, buka modal wajah, lalu ambil 3–5 foto kamera untuk pendaftaran."
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
        <CardDescription>{{ total }} siswa · absensi otomatis belum tersedia.</CardDescription>
      </CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <DataTableToolbar @submit.prevent="filter">
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
        </DataTableToolbar>
        <DataTable
          :data="students ?? []"
          :columns="[
            { id: 'nis', label: 'NIS' },
            { id: 'nama', label: 'Nama', cellClass: 'font-medium' },
            { id: 'kelas', label: 'Kelas' },
            { id: 'status', label: 'Status' },
            {
              id: 'tindakan',
              label: 'Tindakan',
              headClass: 'w-24 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Siswa untuk pengelolaan wajah"
          row-key="id"
          :loading="loading"
        >
          <template #cell-nis="{ item: student }">{{ student.nis }}</template>
          <template #cell-nama="{ item: student }">{{ student.full_name }}</template>
          <template #cell-kelas="{ item: student }">
            {{ student.class_name ?? 'Belum ditempatkan' }}
          </template>
          <template #cell-status="{ item: student }">
            <Badge :variant="statusVariant(student.status)">
              {{ statusLabels[student.status] }}
            </Badge>
          </template>
          <template #cell-tindakan="{ item: student }">
            <TableActions
              :label="student.full_name"
              :disabled="loading"
              :actions="[
                {
                  label: 'Kelola wajah',
                  icon: 'face',
                  onSelect: () => {
                    selected = student
                  },
                },
              ]"
            />
          </template>
          <template #empty>
            {{ loading ? 'Memuat siswa…' : 'Tidak ada siswa sesuai pencarian.' }}
          </template>
        </DataTable>
        <TablePagination
          :page="page"
          :total="total"
          label="siswa"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardContent>
    </Card>
    <StudentFaceEnrollment
      v-if="selected"
      :open="!!selected"
      :student-id="selected.id"
      :student-name="selected.full_name"
      @update:open="
        (value) => {
          if (!value) selected = null
        }
      "
    />
  </div>
</template>
