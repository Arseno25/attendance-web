<script setup lang="ts">
import { toast } from 'vue-sonner'
import {
  academicYearSchema,
  classSchema,
  newClassSchema,
  assignmentSchema,
} from '~/lib/validation/master'
import { statusVariant } from '~/lib/status'
import { Plus } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { AcademicYear, Assignment, ClassAssignmentUser, SchoolClass } from '~/types/master'

definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, canWrite, options, dateTime } = useMasterData()
const auth = useAuth()
const canManageAssignments = computed(
  () => canWrite.value && auth.session.value?.user.scope === 'school',
)
const tab = ref('years')
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const open = ref(false)
const editing = ref('')
const years = ref<AcademicYear[]>([])
const classes = ref<SchoolClass[]>([])
const assignments = ref<Assignment[]>([])
const yearOptions = ref<AcademicYear[]>([])
const classOptions = ref<SchoolClass[]>([])
const assignedUsers = ref<ClassAssignmentUser[]>([])
const revoke = ref<Assignment | null>(null)
const form = reactive({
  name: '',
  starts_on: '',
  ends_on: '',
  is_active: false,
  code: '',
  level: '',
  academic_year_id: '',
  class_id: '',
  user_id: '',
})
const { errors, validate, clear } = useFormValidation(
  () =>
    tab.value === 'years'
      ? academicYearSchema
      : tab.value === 'classes'
        ? editing.value
          ? classSchema
          : newClassSchema
        : assignmentSchema,
  () => form,
)
const labels: Record<string, string> = {
  years: 'tahun ajaran',
  classes: 'kelas',
  assignments: 'penugasan kelas',
}
const paths: Record<string, string> = {
  years: '/academic-years',
  classes: '/classes',
  assignments: '/class-assignments',
}
let loadSequence = 0
async function load() {
  const sequence = ++loadSequence
  const kind = tab.value
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<AcademicYear[] | SchoolClass[] | Assignment[]>>(
      paths[kind]!,
      { query: { page: page.value, page_size: 20 } },
    )
    if (sequence !== loadSequence) return
    if (kind === 'years') years.value = result.data as AcademicYear[]
    else if (kind === 'classes') classes.value = result.data as SchoolClass[]
    else assignments.value = result.data as Assignment[]
    total.value = result.pagination?.total ?? 0
  } catch (error) {
    if (sequence === loadSequence) message.value = apiMessage(error)
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}
watch(tab, async () => {
  clear()
  page.value = 1
  await load()
})
async function show(item?: AcademicYear | SchoolClass) {
  clear()
  message.value = ''

  editing.value = item?.id ?? ''
  Object.assign(
    form,
    {
      name: '',
      starts_on: '',
      ends_on: '',
      is_active: tab.value === 'classes',
      code: '',
      level: '',
      academic_year_id: '',
      class_id: '',
      user_id: '',
    },
    item ?? {},
  )
  try {
    if (tab.value === 'classes') yearOptions.value = await options<AcademicYear>('/academic-years')
    if (tab.value === 'assignments') {
      ;[classOptions.value, assignedUsers.value] = await Promise.all([
        options<SchoolClass>('/classes', { active: 'true' }),
        options<ClassAssignmentUser>('/class-assignment-users'),
      ])
    }
    open.value = true
  } catch (error) {
    message.value = apiMessage(error)
  }
}
async function save() {
  if (saving.value || !validate()) return
  saving.value = true
  message.value = ''
  try {
    const body =
      tab.value === 'years'
        ? {
            name: form.name,
            starts_on: form.starts_on,
            ends_on: form.ends_on,
            is_active: form.is_active,
          }
        : tab.value === 'classes'
          ? {
              name: form.name,
              code: form.code,
              level: form.level,
              is_active: form.is_active,
              ...(!editing.value ? { academic_year_id: form.academic_year_id } : {}),
            }
          : { user_id: form.user_id, class_id: form.class_id }
    await request(`${paths[tab.value]}${editing.value ? `/${editing.value}` : ''}`, {
      method: editing.value ? 'PATCH' : 'POST',
      body,
    })
    open.value = false
    toast.success('Data berhasil disimpan.')
    await load()
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function revokeAssignment() {
  if (!revoke.value) return
  saving.value = true
  message.value = ''
  try {
    await request(`/class-assignments/${revoke.value.id}`, {
      method: 'PATCH',
      body: { is_active: false },
    })
    revoke.value = null
    toast.success('Penugasan dicabut. Akses kelas guru diperbarui segera.')
    await load()
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function changePage(value: number) {
  page.value = value
  await load()
}
await load()
function openRevocation(item: Assignment) {
  message.value = ''
  revoke.value = item
}
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Data akademik"
      :description="
        canWrite
          ? 'Kelola tahun ajaran, kelas, dan penugasan kelas.'
          : 'Kelas dan penugasan yang dapat Anda akses.'
      "
    >
      <template #actions>
        <Button
          v-if="canWrite && (tab !== 'assignments' || canManageAssignments)"
          @click="show()"
        >
          <Plus />
          Tambah {{ labels[tab] }}
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="message && !open && !revoke"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Tabs
      v-model="tab"
      class="min-w-0 gap-4"
    >
      <TabsList>
        <TabsTrigger
          value="years"
          :disabled="loading"
        >
          Tahun ajaran
        </TabsTrigger>
        <TabsTrigger
          value="classes"
          :disabled="loading"
        >
          Kelas
        </TabsTrigger>
        <TabsTrigger
          value="assignments"
          :disabled="loading"
        >
          Penugasan kelas
        </TabsTrigger>
      </TabsList>
      <Card class="min-w-0">
        <CardHeader class="flex flex-row flex-wrap items-center justify-between gap-4">
          <div>
            <CardTitle class="capitalize">{{ labels[tab] }}</CardTitle>
            <CardDescription class="mt-1.5">
              {{ total }} {{ labels[tab] }} tercatat.
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent class="min-w-0">
          <div
            v-if="loading"
            class="space-y-3"
            role="status"
          >
            <span class="sr-only">Memuat data akademik</span>
            <Skeleton
              v-for="n in 4"
              :key="n"
              class="h-10"
            />
          </div>
          <TabsContent
            v-for="kind in ['years', 'classes', 'assignments']"
            v-else
            :key="kind"
            :value="kind"
            class="mt-0"
          >
            <DataTable
              v-if="kind === 'years'"
              class="min-w-[620px]"
              :data="years ?? []"
              :columns="[
                {
                  id: 'tahun-ajaran',
                  label: 'Tahun ajaran',
                  cellClass: 'font-medium',
                },
                {
                  id: 'periode',
                  label: 'Periode',
                },
                { id: 'status', label: 'Status' },
                {
                  id: 'tindakan',
                  label: 'Tindakan',
                  headClass: 'w-24 text-right',
                  cellClass: 'text-right',
                  hidden: !canWrite,
                },
              ]"
              :caption="`Daftar ${labels[kind]}`"
              row-key="id"
              :loading="loading"
            >
              <template #cell-tahun-ajaran="{ item: year }">{{ year.name }}</template>
              <template #cell-periode="{ item: year }">
                {{ year.starts_on }} – {{ year.ends_on }}
              </template>
              <template #cell-status="{ item: year }">
                <Badge :variant="statusVariant(year.is_active ? 'active' : 'inactive')">
                  {{ year.is_active ? 'Aktif' : 'Nonaktif' }}
                </Badge>
              </template>
              <template #cell-tindakan="{ item: year }">
                <TableActions
                  :label="year.name"
                  :disabled="saving"
                  :actions="[
                    {
                      label: 'Edit tahun ajaran',
                      icon: 'edit',
                      onSelect: () => {
                        show(year)
                      },
                    },
                  ]"
                />
              </template>
              <template #empty>
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>Belum ada {{ labels[kind] }}</EmptyTitle>
                    <EmptyDescription>
                      {{
                        canWrite
                          ? 'Tambahkan data untuk memulai pengelolaan sekolah.'
                          : 'Minta admin sekolah mengatur penugasan kelas Anda.'
                      }}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </template>
            </DataTable>
            <DataTable
              v-else-if="kind === 'classes'"
              class="min-w-[620px]"
              :data="classes ?? []"
              :columns="[
                {
                  id: 'kelas',
                  label: 'Kelas',
                },
                {
                  id: 'tahun-ajaran',
                  label: 'Tahun ajaran',
                },
                { id: 'status', label: 'Status' },
                {
                  id: 'tindakan',
                  label: 'Tindakan',
                  headClass: 'w-24 text-right',
                  cellClass: 'text-right',
                  hidden: !canWrite,
                },
              ]"
              :caption="`Daftar ${labels[kind]}`"
              row-key="id"
              :loading="loading"
            >
              <template #cell-kelas="{ item }">
                <div class="font-medium">{{ item.name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ item.code }} · {{ item.level || 'Tingkat belum diisi' }}
                </div>
              </template>
              <template #cell-tahun-ajaran="{ item }">{{ item.academic_year_name }}</template>
              <template #cell-status="{ item }">
                <Badge :variant="statusVariant(item.is_active ? 'active' : 'inactive')">
                  {{ item.is_active ? 'Aktif' : 'Nonaktif' }}
                </Badge>
              </template>
              <template #cell-tindakan="{ item }">
                <TableActions
                  :label="item.name"
                  :disabled="saving"
                  :actions="[
                    {
                      label: 'Edit kelas',
                      icon: 'edit',
                      onSelect: () => {
                        show(item)
                      },
                    },
                  ]"
                />
              </template>
              <template #empty>
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>Belum ada {{ labels[kind] }}</EmptyTitle>
                    <EmptyDescription>
                      {{
                        canWrite
                          ? 'Tambahkan data untuk memulai pengelolaan sekolah.'
                          : 'Minta admin sekolah mengatur penugasan kelas Anda.'
                      }}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </template>
            </DataTable>
            <DataTable
              v-else
              class="min-w-[620px]"
              :data="assignments ?? []"
              :columns="[
                {
                  id: 'pengguna',
                  label: 'Pengguna',
                },
                {
                  id: 'kelas-tahun-ajaran',
                  label: 'Kelas / tahun ajaran',
                },
                { id: 'status', label: 'Status' },
                {
                  id: 'tindakan',
                  label: 'Tindakan',
                  headClass: 'w-24 text-right',
                  cellClass: 'text-right',
                  hidden: !canManageAssignments,
                },
              ]"
              :caption="`Daftar ${labels[kind]}`"
              row-key="id"
              :loading="loading"
            >
              <template #cell-pengguna="{ item }">
                <div class="font-medium">{{ item.user_name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ dateTime(item.starts_at) }} – {{ dateTime(item.ends_at) }}
                </div>
              </template>
              <template #cell-kelas-tahun-ajaran="{ item }">
                {{ item.class_name }}
                <div class="text-xs text-muted-foreground">
                  {{ item.academic_year_name }}
                </div>
              </template>
              <template #cell-status="{ item }">
                <Badge
                  :variant="
                    statusVariant(
                      !item.is_active ? 'revoked' : item.is_current ? 'active' : 'out_of_period',
                    )
                  "
                >
                  {{ !item.is_active ? 'Dicabut' : item.is_current ? 'Aktif' : 'Di luar periode' }}
                </Badge>
              </template>
              <template #cell-tindakan="{ item }">
                <TableActions
                  v-if="item.is_active"
                  :label="item.user_name"
                  :disabled="saving"
                  :actions="[
                    {
                      label: 'Cabut penugasan',
                      icon: 'revoke',
                      onSelect: () => {
                        openRevocation(item)
                      },
                      destructive: true,
                    },
                  ]"
                />
                <span
                  v-else
                  class="text-muted-foreground"
                >
                  —
                </span>
              </template>
              <template #empty>
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>Belum ada {{ labels[kind] }}</EmptyTitle>
                    <EmptyDescription>
                      {{
                        canManageAssignments
                          ? 'Tambahkan data untuk memulai pengelolaan sekolah.'
                          : 'Minta admin sekolah mengatur penugasan kelas Anda.'
                      }}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </template>
            </DataTable>
          </TabsContent>
        </CardContent>
        <CardFooter class="block border-t">
          <TablePagination
            :page="page"
            :total="total"
            :label="labels[tab] ?? 'data'"
            :disabled="loading"
            @update:page="changePage"
          />
        </CardFooter>
      </Card>
    </Tabs>
    <Dialog
      :open="open"
      @update:open="
        (value) => {
          if (!saving) open = value
        }
      "
    >
      <DialogContent
        :show-close-button="!saving"
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>{{ editing ? 'Edit' : 'Tambah' }} {{ labels[tab] }}</DialogTitle>
          <DialogDescription>
            {{
              tab === 'years'
                ? 'Tanggal dikunci setelah kelas dibuat. Aktivasi menggantikan tahun yang sebelumnya aktif.'
                : tab === 'classes'
                  ? 'Kelas selalu terikat pada satu tahun ajaran.'
                  : 'Pengguna dengan peran berscope kelas mendapat akses selama penugasan aktif.'
            }}
          </DialogDescription>
        </DialogHeader>
        <form
          novalidate
          class="flex flex-col gap-6"
          @submit.prevent="save"
        >
          <Alert
            v-if="message"
            variant="destructive"
          >
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <FieldGroup>
            <template v-if="tab === 'years'">
              <Field :data-invalid="!!errors.name">
                <FieldLabel for="year-name">Nama tahun ajaran</FieldLabel>
                <Input
                  id="year-name"
                  v-model="form.name"
                  :aria-invalid="!!errors.name"
                  :aria-describedby="errors.name ? 'year-name-error' : undefined"
                  required
                  maxlength="120"
                  :disabled="saving"
                  placeholder="2026/2027"
                />
                <FieldError
                  v-if="errors.name"
                  id="year-name-error"
                >
                  {{ errors.name }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.starts_on">
                <FieldLabel for="year-start">Tanggal mulai</FieldLabel>
                <Input
                  id="year-start"
                  v-model="form.starts_on"
                  :aria-invalid="!!errors.starts_on"
                  :aria-describedby="errors.starts_on ? 'year-start-error' : undefined"
                  type="date"
                  required
                  :disabled="saving"
                />
                <FieldError
                  v-if="errors.starts_on"
                  id="year-start-error"
                >
                  {{ errors.starts_on }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.ends_on">
                <FieldLabel for="year-end">Tanggal akhir</FieldLabel>
                <Input
                  id="year-end"
                  v-model="form.ends_on"
                  :aria-invalid="!!errors.ends_on"
                  :aria-describedby="errors.ends_on ? 'year-end-error' : undefined"
                  type="date"
                  required
                  :min="form.starts_on"
                  :disabled="saving"
                />
                <FieldError
                  v-if="errors.ends_on"
                  id="year-end-error"
                >
                  {{ errors.ends_on }}
                </FieldError>
              </Field>
            </template>
            <template v-else-if="tab === 'classes'">
              <Field
                v-if="!editing"
                :data-invalid="!!errors.academic_year_id"
              >
                <FieldLabel for="class-year">Tahun ajaran</FieldLabel>
                <Select
                  v-model="form.academic_year_id"
                  :disabled="saving"
                >
                  <SelectTrigger
                    id="class-year"
                    :aria-invalid="!!errors.academic_year_id"
                    :aria-describedby="errors.academic_year_id ? 'class-year-error' : undefined"
                    class="w-full"
                  >
                    <SelectValue placeholder="Pilih tahun ajaran" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="year in yearOptions"
                      :key="year.id"
                      :value="year.id"
                    >
                      {{ year.name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription v-if="!yearOptions.length">
                  Tambahkan tahun ajaran terlebih dahulu.
                </FieldDescription>
                <FieldError
                  v-if="errors.academic_year_id"
                  id="class-year-error"
                >
                  {{ errors.academic_year_id }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.code">
                <FieldLabel for="class-code">Kode kelas</FieldLabel>
                <Input
                  id="class-code"
                  v-model="form.code"
                  :aria-invalid="!!errors.code"
                  :aria-describedby="errors.code ? 'class-code-error' : undefined"
                  required
                  maxlength="30"
                  :disabled="saving"
                  placeholder="7A"
                />
                <FieldError
                  v-if="errors.code"
                  id="class-code-error"
                >
                  {{ errors.code }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.name">
                <FieldLabel for="class-name">Nama kelas</FieldLabel>
                <Input
                  id="class-name"
                  v-model="form.name"
                  :aria-invalid="!!errors.name"
                  :aria-describedby="errors.name ? 'class-name-error' : undefined"
                  required
                  maxlength="120"
                  :disabled="saving"
                  placeholder="Kelas 7 A"
                />
                <FieldError
                  v-if="errors.name"
                  id="class-name-error"
                >
                  {{ errors.name }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.level">
                <FieldLabel for="class-level">Tingkat (opsional)</FieldLabel>
                <Input
                  id="class-level"
                  v-model="form.level"
                  :aria-invalid="!!errors.level"
                  :aria-describedby="errors.level ? 'class-level-error' : undefined"
                  maxlength="30"
                  :disabled="saving"
                  placeholder="7"
                />
                <FieldError
                  v-if="errors.level"
                  id="class-level-error"
                >
                  {{ errors.level }}
                </FieldError>
              </Field>
            </template>
            <template v-else>
              <Field :data-invalid="!!errors.user_id">
                <FieldLabel for="assignment-user">Pengguna dengan peran kelas</FieldLabel>
                <Select
                  v-model="form.user_id"
                  :disabled="saving"
                >
                  <SelectTrigger
                    id="assignment-user"
                    :aria-invalid="!!errors.user_id"
                    :aria-describedby="errors.user_id ? 'assignment-user-error' : undefined"
                    class="w-full"
                  >
                    <SelectValue placeholder="Pilih pengguna aktif" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="user in assignedUsers"
                      :key="user.id"
                      :value="user.id"
                    >
                      {{ user.name }} · {{ user.role }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription v-if="!assignedUsers.length">
                  Buat pengguna aktif dengan peran berscope kelas terlebih dahulu.
                </FieldDescription>
                <FieldError
                  v-if="errors.user_id"
                  id="assignment-user-error"
                >
                  {{ errors.user_id }}
                </FieldError>
              </Field>
              <Field :data-invalid="!!errors.class_id">
                <FieldLabel for="assignment-class">Kelas</FieldLabel>
                <Select
                  v-model="form.class_id"
                  :disabled="saving"
                >
                  <SelectTrigger
                    id="assignment-class"
                    :aria-invalid="!!errors.class_id"
                    :aria-describedby="errors.class_id ? 'assignment-class-error' : undefined"
                    class="w-full"
                  >
                    <SelectValue placeholder="Pilih kelas aktif" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="item in classOptions"
                      :key="item.id"
                      :value="item.id"
                    >
                      {{ item.name }} · {{ item.academic_year_name }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>
                  Penugasan mulai sekarang sampai akhir tahun ajaran.
                </FieldDescription>
                <FieldError
                  v-if="errors.class_id"
                  id="assignment-class-error"
                >
                  {{ errors.class_id }}
                </FieldError>
              </Field>
            </template>
            <Field
              v-if="tab !== 'assignments'"
              orientation="horizontal"
            >
              <Switch
                id="master-active"
                v-model="form.is_active"
                :disabled="saving"
              />
              <FieldLabel for="master-active">
                {{ tab === 'years' ? 'Jadikan tahun ajaran aktif' : 'Kelas aktif' }}
              </FieldLabel>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="saving"
              @click="open = false"
            >
              Batal
            </Button>
            <Button
              type="submit"
              :disabled="saving"
            >
              <Spinner v-if="saving" />
              Simpan {{ labels[tab] }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <AlertDialog
      :open="!!revoke"
      @update:open="
        (value) => {
          if (!value && !saving) revoke = null
        }
      "
    >
      <AlertDialogContent
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        @escape-key-down="
          (event) => {
            if (saving) event.preventDefault()
          }
        "
      >
        <AlertDialogHeader>
          <AlertDialogTitle>Cabut penugasan kelas?</AlertDialogTitle>
          <AlertDialogDescription>
            {{ revoke?.user_name }} akan kehilangan akses ke {{ revoke?.class_name }} dari penugasan
            ini. Histori penugasan tetap tersimpan.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Alert
          v-if="message"
          variant="destructive"
        >
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
        <AlertDialogFooter>
          <AlertDialogCancel as-child>
            <Button
              variant="outline"
              :disabled="saving"
              @click="revoke = null"
            >
              Batal
            </Button>
          </AlertDialogCancel>
          <Button
            variant="destructive"
            :disabled="saving"
            @click="revokeAssignment"
          >
            <Spinner v-if="saving" />
            Cabut penugasan
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
