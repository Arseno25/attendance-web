<script setup lang="ts">
import { toast } from 'vue-sonner'
import { Plus } from '@lucide/vue'
import { roleSchema } from '~/lib/validation/roles'
import type { Permission, Result, RoleRecord, RoleScope } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'super-admin'] })
const request = useApi()
const roles = ref<RoleRecord[]>([])
const permissions = ref<Permission[]>([])
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const editor = ref(false)
const editing = ref<string | null>(null)
const deleting = ref<RoleRecord | null>(null)
const form = reactive({
  name: '',
  description: '',
  scope: 'school' as RoleScope,
  permissions: [] as string[],
})
const { errors, validate, clear } = useFormValidation(
  () => roleSchema,
  () => form,
)
const mandatory = ['users.read', 'users.write', 'roles.manage']
const groups = computed(() => [...new Set(permissions.value.map((permission) => permission.group))])
const groupedPermissions = computed(() =>
  groups.value.map((group) => ({
    group,
    items: permissions.value.filter((permission) => permission.group === group),
  })),
)

async function load() {
  loading.value = true
  message.value = ''
  try {
    const [roleResult, permissionResult] = await Promise.all([
      request<Result<RoleRecord[]>>('/roles'),
      request<Result<Permission[]>>('/permissions'),
    ])
    roles.value = roleResult.data
    permissions.value = permissionResult.data
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    loading.value = false
  }
}
function reset() {
  editor.value = false
  editing.value = null
  clear()
  message.value = ''
  Object.assign(form, { name: '', description: '', scope: 'school', permissions: [] })
}
function edit(role: RoleRecord) {
  clear()
  message.value = ''
  editing.value = role.id
  Object.assign(form, {
    name: role.name,
    description: role.description,
    scope: role.scope,
    permissions: [...role.permissions],
  })
  editor.value = true
}
function togglePermission(key: string, checked: boolean | 'indeterminate') {
  if (editingRole.value?.key === 'super_admin' && mandatory.includes(key)) return
  form.permissions =
    checked === true
      ? [...new Set([...form.permissions, key])]
      : form.permissions.filter((permission) => permission !== key)
}
const editingRole = computed(() => roles.value.find((role) => role.id === editing.value))
function openNew() {
  reset()
  editor.value = true
}
async function save() {
  if (saving.value || !validate()) return
  const isSuperadmin = editingRole.value?.key === 'super_admin'
  const payload = {
    ...form,
    permissions: isSuperadmin
      ? [...new Set([...form.permissions, ...mandatory])]
      : form.permissions,
    scope: isSuperadmin ? 'school' : form.scope,
  }
  saving.value = true
  message.value = ''
  try {
    await request(editing.value ? `/roles/${editing.value}` : '/roles', {
      method: editing.value ? 'PATCH' : 'POST',
      body: payload,
    })
    toast.success('Peran disimpan. Pengguna terkait perlu masuk kembali.')
    reset()
    await load()
  } catch (error) {
    message.value = apiMessage(error)
    toast.error(message.value)
  } finally {
    saving.value = false
  }
}
async function removeRole() {
  if (!deleting.value || saving.value) return
  saving.value = true
  message.value = ''
  try {
    await request(`/roles/${deleting.value.id}`, { method: 'DELETE' })
    toast.success('Peran berhasil dihapus.')
    deleting.value = null
    await load()
  } catch (error) {
    message.value = apiMessage(error)
    toast.error(message.value)
  } finally {
    saving.value = false
  }
}
await load()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Peran & akses"
      description="Atur akses halaman dan cakupan kelas untuk setiap peran."
    >
      <template #actions>
        <Button @click="openNew">
          <Plus />
          Tambah peran
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
    <Card class="min-w-0">
      <CardHeader>
        <CardTitle>Peran sekolah</CardTitle>
        <CardDescription>
          Perubahan hak akses berlaku untuk semua pemegang peran dan mencabut sesi mereka.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div
          v-if="loading"
          class="space-y-3"
          role="status"
        >
          <span class="sr-only">Memuat peran…</span>
          <Skeleton
            v-for="row in 4"
            :key="row"
            class="h-10 w-full"
          />
        </div>
        <DataTable
          v-else
          class="min-w-[700px]"
          :data="roles"
          :columns="[
            { id: 'role', label: 'Peran', cellClass: 'font-medium' },
            { id: 'scope', label: 'Cakupan' },
            { id: 'users', label: 'Pengguna' },
            { id: 'permissions', label: 'Hak akses' },
            {
              id: 'actions',
              label: 'Tindakan',
              headClass: 'w-28 text-right',
              cellClass: 'text-right',
            },
          ]"
          caption="Peran dan hak akses sekolah"
          row-key="id"
        >
          <template #cell-role="{ item }">
            <div>
              {{ item.name }}
              <Badge
                v-if="item.is_system"
                variant="secondary"
              >
                Sistem
              </Badge>
            </div>
            <div class="text-xs text-muted-foreground">{{ item.description || item.key }}</div>
          </template>
          <template #cell-scope="{ item }">
            {{ item.scope === 'school' ? 'Seluruh sekolah' : 'Kelas ditugaskan' }}
          </template>
          <template #cell-users="{ item }">{{ item.user_count }}</template>
          <template #cell-permissions="{ item }">{{ item.permissions.length }}</template>
          <template #cell-actions="{ item }">
            <div class="flex justify-end gap-2">
              <Button
                size="sm"
                variant="outline"
                @click="edit(item)"
              >
                Edit
              </Button>
              <Button
                v-if="!item.is_system"
                size="sm"
                variant="destructive"
                :disabled="saving"
                @click="deleting = item"
              >
                Hapus
              </Button>
            </div>
          </template>
          <template #empty>
            <Empty>
              <EmptyHeader>
                <EmptyTitle>Belum ada peran</EmptyTitle>
                <EmptyDescription>Buat peran untuk mengatur hak akses pengguna.</EmptyDescription>
              </EmptyHeader>
            </Empty>
          </template>
        </DataTable>
      </CardContent>
    </Card>
    <Dialog
      :open="editor"
      @update:open="
        (value) => {
          if (!saving && !value) reset()
        }
      "
    >
      <DialogContent
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
        :show-close-button="!saving"
      >
        <DialogHeader>
          <DialogTitle>{{ editing ? 'Edit peran' : 'Tambah peran' }}</DialogTitle>
          <DialogDescription>
            Hak akses peran berlaku untuk semua pengguna yang ditetapkan. Sesi mereka akan dicabut
            setelah disimpan.
          </DialogDescription>
        </DialogHeader>
        <form
          class="flex flex-col gap-5"
          novalidate
          @submit.prevent="save"
        >
          <Alert
            v-if="message"
            variant="destructive"
            role="alert"
          >
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <FieldGroup class="grid gap-4 sm:grid-cols-2">
            <Field :data-invalid="!!errors.name">
              <FieldLabel for="role-name">Nama peran</FieldLabel>
              <Input
                id="role-name"
                v-model="form.name"
                maxlength="120"
                :disabled="saving"
                :aria-invalid="!!errors.name"
                :aria-describedby="errors.name ? 'role-name-error' : undefined"
              />
              <FieldError
                v-if="errors.name"
                id="role-name-error"
              >
                {{ errors.name }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.scope">
              <FieldLabel for="role-scope">Cakupan data</FieldLabel>
              <Select
                v-model="form.scope"
                :disabled="saving || editingRole?.key === 'super_admin'"
              >
                <SelectTrigger
                  id="role-scope"
                  class="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="school">Seluruh sekolah</SelectItem>
                  <SelectItem value="assigned_classes">Kelas yang ditugaskan</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field
              class="sm:col-span-2"
              :data-invalid="!!errors.description"
            >
              <FieldLabel for="role-description">Deskripsi</FieldLabel>
              <Textarea
                id="role-description"
                v-model="form.description"
                maxlength="500"
                :disabled="saving"
              />
              <FieldError v-if="errors.description">{{ errors.description }}</FieldError>
            </Field>
          </FieldGroup>
          <FieldSet :data-invalid="!!errors.permissions">
            <FieldLegend variant="label">Hak akses</FieldLegend>
            <FieldDescription>
              Pilih hanya kemampuan yang diperlukan. Cakupan kelas tetap berlaku di semua data
              kelas.
            </FieldDescription>
            <div
              v-for="group in groupedPermissions"
              :key="group.group"
              class="mt-4 space-y-2"
            >
              <h3 class="text-sm font-semibold">{{ group.group }}</h3>
              <FieldGroup class="grid gap-3 sm:grid-cols-2">
                <Field
                  v-for="permission in group.items"
                  :key="permission.key"
                  orientation="horizontal"
                >
                  <Checkbox
                    :id="`role-${permission.key}`"
                    :model-value="form.permissions.includes(permission.key)"
                    :disabled="
                      saving ||
                      (editingRole?.key === 'super_admin' && mandatory.includes(permission.key))
                    "
                    @update:model-value="(checked) => togglePermission(permission.key, checked)"
                  />
                  <FieldContent>
                    <FieldLabel
                      :for="`role-${permission.key}`"
                      class="font-medium"
                    >
                      {{ permission.label }}
                      <Badge
                        v-if="
                          editingRole?.key === 'super_admin' && mandatory.includes(permission.key)
                        "
                        variant="secondary"
                      >
                        Wajib
                      </Badge>
                    </FieldLabel>
                    <FieldDescription>{{ permission.description }}</FieldDescription>
                  </FieldContent>
                </Field>
              </FieldGroup>
            </div>
          </FieldSet>
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
              {{ saving ? 'Menyimpan…' : 'Simpan peran' }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <AlertDialog
      :open="!!deleting"
      @update:open="
        (value) => {
          if (!value && !saving) deleting = null
        }
      "
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Hapus peran?</AlertDialogTitle>
          <AlertDialogDescription>
            Peran {{ deleting?.name }} digunakan {{ deleting?.user_count }} akun. Pindahkan akun
            tersebut ke peran lain sebelum menghapusnya.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Alert
          v-if="message"
          variant="destructive"
        >
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="saving">Batal</AlertDialogCancel>
          <Button
            variant="destructive"
            :disabled="saving"
            @click="removeRole"
          >
            <Spinner v-if="saving" />
            Hapus peran
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
