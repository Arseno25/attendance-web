<script setup lang="ts">
import { studentFilterSchema } from '~/lib/validation/master'
import { statusVariant } from '~/lib/status'
import { Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { statusLabels, type Student, type SchoolClass } from '~/types/master'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canWrite } = useMasterData()
const students = ref<Student[]>([])
const classes = ref<SchoolClass[]>([])
const page = ref(1)
const total = ref(0)
const search = ref('')
const classFilter = ref('all')
const status = ref('all')
const loading = ref(false)
const message = ref('')
const success = ref('')
const open = ref(false)
const { errors, validate } = useFormValidation(
  () => studentFilterSchema,
  () => ({
    search: search.value,
    class_id: classFilter.value === 'all' ? '' : classFilter.value,
    status: status.value === 'all' ? '' : status.value,
  }),
)
async function load() {
  if (!validate()) return
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<Student[]>>('/students', {
      query: {
        page: page.value,
        page_size: 20,
        search: search.value,
        class_id: classFilter.value === 'all' ? '' : classFilter.value,
        status: status.value === 'all' ? '' : status.value,
      },
    })
    students.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (error) {
    message.value = apiMessage(error)
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
async function saved() {
  success.value = 'Siswa berhasil ditambahkan.'
  page.value = 1
  await load()
}
try {
  classes.value = await options<SchoolClass>('/classes', { active: 'true' })
} catch (error) {
  message.value = apiMessage(error)
}
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Siswa"
      :description="
        canWrite
          ? 'Data siswa dan histori penempatan kelas.'
          : 'Siswa aktif pada kelas yang ditugaskan kepada Anda.'
      "
    >
      <template #actions>
        <div
          v-if="canWrite"
          class="flex flex-wrap gap-2"
        >
          <Button
            as-child
            variant="outline"
          >
            <NuxtLink to="/admin/students/import">Import CSV</NuxtLink>
          </Button>
          <Button @click="open = true">
            <Plus />
            Tambah siswa
          </Button>
        </div>
      </template>
    </PageHeader>
    <Alert
      v-if="message"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Alert
      v-if="success"
      role="status"
    >
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Daftar siswa</CardTitle>
        <CardDescription>{{ total }} siswa sesuai filter.</CardDescription>
      </CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <form
          novalidate
          class="grid gap-3 sm:grid-cols-2 xl:flex"
          @submit.prevent="filter"
        >
          <Label
            for="student-search"
            class="sr-only"
          >
            Cari nama atau NIS
          </Label>
          <Field
            class="xl:max-w-sm xl:flex-1"
            :data-invalid="!!errors.search"
          >
            <Input
              id="student-search"
              v-model="search"
              :aria-invalid="!!errors.search"
              :aria-describedby="errors.search ? 'student-search-error' : undefined"
              class="xl:max-w-sm"
              placeholder="Cari nama atau NIS"
              maxlength="100"
            />
            <FieldError
              v-if="errors.search"
              id="student-search-error"
            >
              {{ errors.search }}
            </FieldError>
          </Field>
          <Field
            class="xl:w-48"
            :data-invalid="!!errors.class_id"
          >
            <Select
              v-model="classFilter"
              :disabled="loading"
              @update:model-value="filter"
            >
              <SelectTrigger
                id="student-class-filter"
                :aria-invalid="!!errors.class_id"
                :aria-describedby="errors.class_id ? 'student-class-filter-error' : undefined"
                aria-label="Filter kelas"
                class="w-full xl:w-48"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua kelas</SelectItem>
                <SelectItem
                  v-for="item in classes"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <FieldError
              v-if="errors.class_id"
              id="student-class-filter-error"
            >
              {{ errors.class_id }}
            </FieldError>
          </Field>
          <Field
            v-if="canWrite"
            class="xl:w-40"
            :data-invalid="!!errors.status"
          >
            <Select
              v-model="status"
              :disabled="loading"
              @update:model-value="filter"
            >
              <SelectTrigger
                id="student-status-filter"
                :aria-invalid="!!errors.status"
                :aria-describedby="errors.status ? 'student-status-filter-error' : undefined"
                aria-label="Filter status"
                class="w-full xl:w-40"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua status</SelectItem>
                <SelectItem
                  v-for="(label, value) in statusLabels"
                  :key="value"
                  :value="value"
                >
                  {{ label }}
                </SelectItem>
              </SelectContent>
            </Select>
            <FieldError
              v-if="errors.status"
              id="student-status-filter-error"
            >
              {{ errors.status }}
            </FieldError>
          </Field>
          <Button
            variant="outline"
            type="submit"
            :disabled="loading"
          >
            Cari
          </Button>
        </form>
        <div
          v-if="loading"
          class="space-y-3"
          role="status"
        >
          <span class="sr-only">Memuat siswa</span>
          <Skeleton
            v-for="n in 5"
            :key="n"
            class="h-10"
          />
        </div>
        <Table
          v-else
          class="min-w-[640px]"
        >
          <TableCaption class="sr-only">Daftar siswa sekolah</TableCaption>
          <TableHeader class="bg-muted/50">
            <TableRow>
              <TableHead>Nama siswa</TableHead>
              <TableHead>NIS</TableHead>
              <TableHead>Kelas terakhir</TableHead>
              <TableHead>Status</TableHead>
              <TableHead class="w-12 text-right">Tindakan</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="student in students"
              :key="student.id"
            >
              <TableCell class="font-medium">
                <NuxtLink
                  :to="`/admin/students/${student.id}`"
                  class="hover:underline"
                >
                  {{ student.full_name }}
                </NuxtLink>
              </TableCell>
              <TableCell>{{ student.nis }}</TableCell>
              <TableCell>
                {{ student.class_name ?? 'Belum ditempatkan' }}
                <div class="text-xs text-muted-foreground">
                  {{ student.academic_year_name }}
                </div>
              </TableCell>
              <TableCell>
                <Badge :variant="statusVariant(student.status)">
                  {{ statusLabels[student.status] }}
                </Badge>
              </TableCell>
              <TableCell class="text-right">
                <TableActions :label="student.full_name">
                  <DropdownMenuItem as-child>
                    <NuxtLink :to="`/admin/students/${student.id}`">Detail</NuxtLink>
                  </DropdownMenuItem>
                </TableActions>
              </TableCell>
            </TableRow>
            <TableRow v-if="!students.length">
              <TableCell colspan="5">
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>Tidak ada siswa sesuai filter</EmptyTitle>
                    <EmptyDescription>
                      Coba pencarian lain atau tambahkan siswa pada kelas aktif.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
      <CardFooter class="block border-t">
        <TablePagination
          :page="page"
          :total="total"
          label="siswa"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardFooter>
    </Card>
    <StudentEditor
      v-if="canWrite"
      v-model:open="open"
      @saved="saved"
    />
  </div>
</template>
