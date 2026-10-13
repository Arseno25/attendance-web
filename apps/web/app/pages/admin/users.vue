<script setup lang="ts">
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
const message = ref('')
const success = ref('')
const form = reactive({ email: '', full_name: '', role: 'teacher' as Role, password: '', is_active: true, grants: [] as string[] })
const grants = [
  { id: 'biometrics.enroll', label: 'Registrasi biometrik' }, { id: 'biometrics.revoke', label: 'Cabut biometrik' },
  { id: 'consents.manage', label: 'Kelola consent' }, { id: 'attendance.manual', label: 'Absensi manual' },
  { id: 'attendance.approve', label: 'Setujui koreksi' }, { id: 'reports.export', label: 'Ekspor laporan' },
]
const availableGrants = computed(() => grants.filter(grant =>
  form.role === 'admin' || form.role === 'super_admin'
  || (form.role === 'operator' && ['biometrics.enroll', 'attendance.manual'].includes(grant.id))
  || (form.role === 'teacher' && grant.id === 'reports.export'),
))
watch(() => form.role, () => { form.grants = form.grants.filter(id => availableGrants.value.some(grant => grant.id === id)) })

async function load() {
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<User[]>>('/users', { query: { page: page.value, page_size: 20, search: search.value } })
    users.value = result.data
    total.value = result.pagination?.total ?? 0
  }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
function reset() {
  editing.value = null
  Object.assign(form, { email: '', full_name: '', role: 'teacher', password: '', is_active: true, grants: [] })
}
function edit(user: User) {
  editing.value = user.id
  Object.assign(form, { email: user.email, full_name: user.full_name, role: user.role, password: '', is_active: user.is_active, grants: [...user.grants] })
  success.value = ''
}
async function save() {
  if (saving.value) return
  saving.value = true
  message.value = ''
  success.value = ''
  try {
    const body = editing.value
      ? { email: form.email, full_name: form.full_name, role: form.role, is_active: form.is_active, grants: form.grants }
      : { email: form.email, full_name: form.full_name, role: form.role, password: form.password, grants: form.grants }
    await request(editing.value ? `/users/${editing.value}` : '/users', { method: editing.value ? 'PATCH' : 'POST', body })
    success.value = 'Pengguna berhasil disimpan.'
    reset()
    await load()
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
async function changePage(value: number) { page.value = value; await load() }
await load()
</script>

<template>
  <div class="flex flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Pengguna & peran</h1><p class="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">Atur akses petugas sekolah. Perubahan akses mencabut sesi pengguna terkait.</p></div>
    <Alert v-if="message" variant="destructive" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Card class="shadow-none">
      <CardHeader><CardTitle>{{ editing ? 'Edit pengguna' : 'Tambah pengguna' }}</CardTitle><CardDescription>{{ editing ? 'Periksa peran dan izin sebelum menyimpan.' : 'Buat akun untuk petugas sekolah.' }}</CardDescription></CardHeader>
      <CardContent>
        <form class="space-y-6" @submit.prevent="save">
          <FieldGroup class="grid gap-5 md:grid-cols-2">
            <Field><FieldLabel for="user-name">Nama</FieldLabel><Input id="user-name" v-model="form.full_name" required maxlength="120" :disabled="saving" /></Field>
            <Field><FieldLabel for="user-email">Email</FieldLabel><Input id="user-email" v-model="form.email" type="email" required maxlength="254" :disabled="saving" /></Field>
            <Field><FieldLabel for="user-role">Peran</FieldLabel><Select :model-value="form.role" :disabled="saving" @update:model-value="value => form.role = value as Role"><SelectTrigger id="user-role" class="w-full"><SelectValue placeholder="Pilih peran" /></SelectTrigger><SelectContent><SelectItem v-for="(label, role) in roleLabels" :key="role" :value="role">{{ label }}</SelectItem></SelectContent></Select></Field>
            <Field v-if="!editing"><FieldLabel for="new-password">Password awal</FieldLabel><Input id="new-password" v-model="form.password" type="password" autocomplete="new-password" required minlength="12" maxlength="128" :disabled="saving" /><FieldDescription>Minimal 12 karakter; tidak dikirim lewat email otomatis.</FieldDescription></Field>
          </FieldGroup>
          <FieldSeparator />
          <FieldSet>
            <FieldLegend variant="label">Izin tambahan</FieldLegend>
            <FieldDescription>Izin tambahan tetap mengikuti batas akses peran yang dipilih.</FieldDescription>
            <FieldGroup class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              <Field v-for="grant in availableGrants" :key="grant.id" orientation="horizontal">
                <Checkbox :id="`grant-${grant.id}`" :model-value="form.grants.includes(grant.id)" :disabled="saving" @update:model-value="checked => form.grants = checked === true ? [...form.grants, grant.id] : form.grants.filter(id => id !== grant.id)" />
                <FieldLabel :for="`grant-${grant.id}`" class="font-normal">{{ grant.label }}</FieldLabel>
              </Field>
            </FieldGroup>
          </FieldSet>
          <Field v-if="editing" orientation="horizontal">
            <Switch id="user-active" v-model="form.is_active" :disabled="saving" />
            <FieldContent><FieldLabel for="user-active">Akun aktif</FieldLabel><FieldDescription>Akun nonaktif tidak dapat masuk ke panel.</FieldDescription></FieldContent>
          </Field>
          <div class="flex flex-wrap gap-3"><Button type="submit" :disabled="saving"><Spinner v-if="saving" />{{ saving ? 'Menyimpan…' : 'Simpan pengguna' }}</Button><Button v-if="editing" type="button" variant="outline" :disabled="saving" @click="reset">Batal</Button></div>
        </form>
      </CardContent>
    </Card>
    <Card class="min-w-0 shadow-none">
      <CardHeader class="gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div><CardTitle>Daftar pengguna</CardTitle><CardDescription class="mt-1.5">Cari dan perbarui akun petugas sekolah.</CardDescription></div>
        <form class="flex w-full gap-2 sm:max-w-sm" @submit.prevent="page = 1; load()"><Label for="user-search" class="sr-only">Cari pengguna</Label><Input id="user-search" v-model="search" placeholder="Cari nama atau email" maxlength="100" /><Button variant="outline" type="submit" :disabled="loading">Cari</Button></form>
      </CardHeader>
      <CardContent class="min-w-0">
        <div v-if="loading" class="space-y-3" role="status"><span class="sr-only">Memuat pengguna…</span><Skeleton v-for="row in 4" :key="row" class="h-10 w-full" /></div>
        <Table v-else class="min-w-[640px]">
          <TableCaption class="sr-only">Daftar pengguna sekolah</TableCaption>
          <TableHeader><TableRow><TableHead>Nama</TableHead><TableHead>Email</TableHead><TableHead>Peran</TableHead><TableHead>Status</TableHead><TableHead class="text-right">Tindakan</TableHead></TableRow></TableHeader>
          <TableBody>
            <TableRow v-for="user in users" :key="user.id"><TableCell class="font-medium">{{ user.full_name }}</TableCell><TableCell>{{ user.email }}</TableCell><TableCell>{{ roleLabels[user.role] }}</TableCell><TableCell><Badge :variant="user.is_active ? 'secondary' : 'outline'">{{ user.is_active ? 'Aktif' : 'Nonaktif' }}</Badge></TableCell><TableCell class="text-right"><Button variant="ghost" size="sm" :disabled="user.id === auth.session.value?.user.id" @click="edit(user)">{{ user.id === auth.session.value?.user.id ? 'Akun Anda' : 'Edit' }}</Button></TableCell></TableRow>
            <TableRow v-if="!users.length"><TableCell colspan="5"><Empty><EmptyHeader><EmptyTitle>Tidak ada pengguna sesuai pencarian</EmptyTitle><EmptyDescription>Coba nama atau email lain, atau kosongkan pencarian untuk melihat semua pengguna.</EmptyDescription></EmptyHeader></Empty></TableCell></TableRow>
          </TableBody>
        </Table>
      </CardContent>
      <CardFooter class="block border-t pt-4"><TablePagination :page="page" :total="total" label="pengguna" :disabled="loading" @update:page="changePage" /></CardFooter>
    </Card>
  </div>
</template>
