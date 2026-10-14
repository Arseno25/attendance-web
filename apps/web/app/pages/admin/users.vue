<script setup lang="ts">
import { toast } from 'vue-sonner'
import { searchSchema } from '~/lib/validation/master'
import { userSchema } from '~/lib/validation/auth'
import { Plus } from '@lucide/vue'
import { statusVariant } from '~/lib/status'
import { roleLabels, type Result, type Role, type User } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'super-admin'] })
const request = useApi()
const auth = useAuth()
const users = ref<User[]>([])
const page = ref(1)
const total = ref(0)
const search = ref('')
const loading = ref(false)
const saving = ref(false)
const editing = ref<string | null>(null)
const editor = ref(false)
const message = ref('')
const form = reactive({
  email: '',
  full_name: '',
  role: 'teacher' as Role,
  password: '',
  is_active: true,
  grants: [] as string[],
})
const { errors, validate, clear } = useFormValidation(
  () => userSchema(!!editing.value),
  () => form,
)
const { errors: filterErrors, validate: validateFilter } = useFormValidation(
  () => searchSchema,
  () => ({ search: search.value }),
)
const grants = [
  { id: 'biometrics.enroll', label: 'Registrasi biometrik' },
  { id: 'biometrics.revoke', label: 'Cabut biometrik' },
  { id: 'consents.manage', label: 'Kelola consent' },
  { id: 'attendance.manual', label: 'Absensi manual' },
  { id: 'attendance.approve', label: 'Setujui koreksi' },
  { id: 'reports.export', label: 'Ekspor laporan' },
]
const availableGrants = computed(() =>
  grants.filter(
    (grant) =>
      form.role === 'admin' ||
      form.role === 'super_admin' ||
      (form.role === 'operator' && ['biometrics.enroll', 'attendance.manual'].includes(grant.id)) ||
      (form.role === 'teacher' && grant.id === 'reports.export'),
  ),
)
watch(
  () => form.role,
  () => {
    form.grants = form.grants.filter((id) => availableGrants.value.some((grant) => grant.id === id))
  },
)

async function load() {
  if (!validateFilter()) return
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<User[]>>('/users', {
      query: { page: page.value, page_size: 20, search: search.value },
    })
    users.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    loading.value = false
  }
}
function reset() {
  clear()
  editor.value = false
  message.value = ''
  editing.value = null
  Object.assign(form, {
    email: '',
    full_name: '',
    role: 'teacher',
    password: '',
    is_active: true,
    grants: [],
  })
}
function edit(user: User) {
  clear()
  message.value = ''
  editor.value = true
  editing.value = user.id
  Object.assign(form, {
    email: user.email,
    full_name: user.full_name,
    role: user.role,
    password: '',
    is_active: user.is_active,
    grants: [...user.grants],
  })
}
async function save() {
  if (saving.value || !validate()) return
  saving.value = true
  message.value = ''

  try {
    const body = editing.value
      ? {
          email: form.email,
          full_name: form.full_name,
          role: form.role,
          is_active: form.is_active,
          grants: form.grants,
        }
      : {
          email: form.email,
          full_name: form.full_name,
          role: form.role,
          password: form.password,
          grants: form.grants,
        }
    await request(editing.value ? `/users/${editing.value}` : '/users', {
      method: editing.value ? 'PATCH' : 'POST',
      body,
    })
    toast.success('Pengguna berhasil disimpan.')
    reset()
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
function openUser() {
  reset()

  editor.value = true
}
async function filter() {
  page.value = 1
  await load()
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <PageHeader
      title="Pengguna & peran"
      description="Atur akses petugas sekolah. Perubahan akses mencabut sesi pengguna terkait."
    >
      <template #actions>
        <Button @click="openUser">
          <Plus />
          Tambah pengguna
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="message && !editor"
      variant="destructive"
      role="alert"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Dialog
      :open="editor"
      @update:open="
        (value) => {
          if (!saving) {
            editor = value
            if (!value) reset()
          }
        }
      "
    >
      <DialogContent
        :show-close-button="!saving"
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
      >
        <DialogHeader>
          <DialogTitle>{{ editing ? 'Edit pengguna' : 'Tambah pengguna' }}</DialogTitle>
          <DialogDescription>
            {{
              editing
                ? 'Periksa peran dan izin sebelum menyimpan.'
                : 'Buat akun untuk petugas sekolah.'
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
          <FieldGroup class="grid gap-5 sm:grid-cols-2">
            <Field :data-invalid="!!errors.full_name">
              <FieldLabel for="user-name">Nama</FieldLabel>
              <Input
                id="user-name"
                v-model="form.full_name"
                :aria-invalid="!!errors.full_name"
                :aria-describedby="errors.full_name ? 'user-name-error' : undefined"
                required
                maxlength="120"
                :disabled="saving"
              />
              <FieldError
                v-if="errors.full_name"
                id="user-name-error"
              >
                {{ errors.full_name }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.email">
              <FieldLabel for="user-email">Email</FieldLabel>
              <Input
                id="user-email"
                v-model="form.email"
                :aria-invalid="!!errors.email"
                :aria-describedby="errors.email ? 'user-email-error' : undefined"
                type="email"
                required
                maxlength="254"
                :disabled="saving"
              />
              <FieldError
                v-if="errors.email"
                id="user-email-error"
              >
                {{ errors.email }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.role">
              <FieldLabel for="user-role">Peran</FieldLabel>
              <Select
                :model-value="form.role"
                :disabled="saving"
                @update:model-value="(value) => (form.role = value as Role)"
              >
                <SelectTrigger
                  id="user-role"
                  :aria-invalid="!!errors.role"
                  :aria-describedby="errors.role ? 'user-role-error' : undefined"
                  class="w-full"
                >
                  <SelectValue placeholder="Pilih peran" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="(label, role) in roleLabels"
                    :key="role"
                    :value="role"
                  >
                    {{ label }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FieldError
                v-if="errors.role"
                id="user-role-error"
              >
                {{ errors.role }}
              </FieldError>
            </Field>
            <Field
              v-if="!editing"
              :data-invalid="!!errors.password"
            >
              <FieldLabel for="new-password">Password awal</FieldLabel>
              <Input
                id="new-password"
                v-model="form.password"
                :aria-invalid="!!errors.password"
                :aria-describedby="
                  errors.password ? 'new-password-help new-password-error' : 'new-password-help'
                "
                type="password"
                autocomplete="new-password"
                required
                minlength="8"
                :disabled="saving"
              />
              <PasswordRequirements
                id="new-password-help"
                :value="form.password"
              />
              <FieldError
                v-if="errors.password"
                id="new-password-error"
              >
                {{ errors.password }}
              </FieldError>
            </Field>
          </FieldGroup>
          <FieldSeparator />
          <FieldSet :data-invalid="!!errors.grants">
            <FieldLegend variant="label">Izin tambahan</FieldLegend>
            <FieldDescription>
              Izin tambahan tetap mengikuti batas akses peran yang dipilih.
            </FieldDescription>
            <FieldGroup class="grid gap-4 sm:grid-cols-2">
              <Field
                v-for="grant in availableGrants"
                :key="grant.id"
                orientation="horizontal"
              >
                <Checkbox
                  :id="`grant-${grant.id}`"
                  :aria-invalid="!!errors.grants"
                  :aria-describedby="errors.grants ? 'user-grants-error' : undefined"
                  :model-value="form.grants.includes(grant.id)"
                  :disabled="saving"
                  @update:model-value="
                    (checked) =>
                      (form.grants =
                        checked === true
                          ? [...form.grants, grant.id]
                          : form.grants.filter((id) => id !== grant.id))
                  "
                />
                <FieldLabel
                  :for="`grant-${grant.id}`"
                  class="font-normal"
                >
                  {{ grant.label }}
                </FieldLabel>
              </Field>
            </FieldGroup>
            <FieldError
              v-if="errors.grants"
              id="user-grants-error"
            >
              {{ errors.grants }}
            </FieldError>
          </FieldSet>
          <Field
            v-if="editing"
            orientation="horizontal"
          >
            <Switch
              id="user-active"
              v-model="form.is_active"
              :disabled="saving"
            />
            <FieldContent>
              <FieldLabel for="user-active">Akun aktif</FieldLabel>
              <FieldDescription>Akun nonaktif tidak dapat masuk ke panel.</FieldDescription>
            </FieldContent>
          </Field>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="saving"
              @click="reset"
            >
              Batal
            </Button>
            <Button
              type="submit"
              :disabled="saving"
            >
              <Spinner v-if="saving" />
              {{ saving ? 'Menyimpan…' : 'Simpan pengguna' }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <Card class="min-w-0">
      <CardHeader>
        <div>
          <CardTitle>Daftar pengguna</CardTitle>
          <CardDescription class="mt-1.5">Cari dan perbarui akun petugas sekolah.</CardDescription>
        </div>
      </CardHeader>
      <CardContent class="min-w-0 space-y-4">
        <DataTableToolbar @submit.prevent="filter">
          <Label
            for="user-search"
            class="sr-only"
          >
            Cari pengguna
          </Label>
          <Field
            class="min-w-0 flex-1 sm:max-w-sm"
            :data-invalid="!!filterErrors.search"
          >
            <Input
              id="user-search"
              v-model="search"
              :aria-invalid="!!filterErrors.search"
              :aria-describedby="filterErrors.search ? 'user-search-error' : undefined"
              placeholder="Cari nama atau email"
              maxlength="100"
            />
            <FieldError
              v-if="filterErrors.search"
              id="user-search-error"
            >
              {{ filterErrors.search }}
            </FieldError>
          </Field>
          <Button
            variant="outline"
            type="submit"
            :disabled="loading"
          >
            Cari
          </Button>
        </DataTableToolbar>
        <div
          v-if="loading"
          class="space-y-3"
          role="status"
        >
          <span class="sr-only">Memuat pengguna…</span>
          <Skeleton
            v-for="row in 4"
            :key="row"
            class="h-10 w-full"
          />
        </div>
        <DataTable
          v-else
          class="min-w-[640px]"
          :data="users ?? []"
          :columns="[
            { id: 'nama', label: 'Nama', cellClass: 'font-medium' },
            { id: 'email', label: 'Email' },
            { id: 'peran', label: 'Peran' },
            { id: 'status', label: 'Status' },
            {
              id: 'tindakan',
              label: 'Tindakan',
              headClass: 'w-24 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Daftar pengguna sekolah"
          row-key="id"
          :loading="loading"
        >
          <template #cell-nama="{ item: user }">{{ user.full_name }}</template>
          <template #cell-email="{ item: user }">{{ user.email }}</template>
          <template #cell-peran="{ item: user }">{{ roleLabels[user.role] }}</template>
          <template #cell-status="{ item: user }">
            <Badge :variant="statusVariant(user.is_active ? 'active' : 'inactive')">
              {{ user.is_active ? 'Aktif' : 'Nonaktif' }}
            </Badge>
          </template>
          <template #cell-tindakan="{ item: user }">
            <TableActions
              :label="user.full_name"
              :disabled="saving || user.id === auth.session.value?.user.id"
              :actions="[
                {
                  label: 'Edit pengguna',
                  icon: 'edit',
                  onSelect: () => {
                    edit(user)
                  },
                },
              ]"
            />
            <span
              v-if="user.id === auth.session.value?.user.id"
              class="sr-only"
            >
              Akun Anda
            </span>
          </template>
          <template #empty>
            <Empty>
              <EmptyHeader>
                <EmptyTitle>Tidak ada pengguna sesuai pencarian</EmptyTitle>
                <EmptyDescription>
                  Coba nama atau email lain, atau kosongkan pencarian untuk melihat semua pengguna.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </template>
        </DataTable>
      </CardContent>
      <CardFooter class="block border-t pt-4">
        <TablePagination
          :page="page"
          :total="total"
          label="pengguna"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardFooter>
    </Card>
  </div>
</template>
