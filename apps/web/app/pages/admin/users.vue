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
async function changePage(delta: number) { page.value += delta; await load() }
await load()
</script>

<template>
  <div class="max-w-6xl">
    <h1 class="text-2xl font-semibold tracking-tight">Pengguna & peran</h1>
    <p class="mt-2 text-sm leading-6 text-muted-foreground">Atur akses petugas sekolah. Perubahan akses mencabut sesi pengguna terkait.</p>
    <Alert v-if="message" class="mt-6" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <p v-if="success" class="mt-4 text-sm" role="status">{{ success }}</p>
    <Card class="mt-6">
      <CardHeader><CardTitle>{{ editing ? 'Edit pengguna' : 'Tambah pengguna' }}</CardTitle><CardDescription>{{ editing ? 'Periksa peran dan izin sebelum menyimpan.' : 'Buat akun untuk petugas sekolah.' }}</CardDescription></CardHeader>
      <CardContent>
        <form class="space-y-5" @submit.prevent="save">
          <div class="grid gap-5 sm:grid-cols-2">
            <div class="space-y-2"><Label for="user-name">Nama</Label><Input id="user-name" v-model="form.full_name" required maxlength="120" :disabled="saving" /></div>
            <div class="space-y-2"><Label for="user-email">Email</Label><Input id="user-email" v-model="form.email" type="email" required maxlength="254" :disabled="saving" /></div>
            <div class="space-y-2"><Label for="user-role">Peran</Label><select id="user-role" v-model="form.role" class="h-9 w-full rounded-md border bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-ring" :disabled="saving"><option v-for="(label, role) in roleLabels" :key="role" :value="role">{{ label }}</option></select></div>
            <div v-if="!editing" class="space-y-2"><Label for="new-password">Password awal</Label><Input id="new-password" v-model="form.password" type="password" autocomplete="new-password" required minlength="12" maxlength="128" :disabled="saving" /><p class="text-xs text-muted-foreground">Minimal 12 karakter; tidak dikirim lewat email otomatis.</p></div>
          </div>
          <fieldset class="space-y-3"><legend class="mb-3 text-sm font-medium">Izin tambahan</legend><div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3"><label v-for="grant in availableGrants" :key="grant.id" class="flex items-center gap-2 text-sm"><input v-model="form.grants" type="checkbox" :value="grant.id" :disabled="saving" class="size-4 accent-primary">{{ grant.label }}</label></div></fieldset>
          <label v-if="editing" class="flex items-center gap-2 text-sm"><input v-model="form.is_active" type="checkbox" :disabled="saving" class="size-4 accent-primary">Akun aktif</label>
          <div class="flex gap-3"><Button type="submit" :disabled="saving">{{ saving ? 'Menyimpan…' : 'Simpan pengguna' }}</Button><Button v-if="editing" type="button" variant="outline" :disabled="saving" @click="reset">Batal</Button></div>
        </form>
      </CardContent>
    </Card>
    <form class="mt-8 flex max-w-md gap-2" @submit.prevent="page = 1; load()"><Label for="user-search" class="sr-only">Cari pengguna</Label><Input id="user-search" v-model="search" placeholder="Cari nama atau email" maxlength="100" /><Button variant="outline" type="submit" :disabled="loading">Cari</Button></form>
    <p v-if="loading" class="mt-6 text-sm" role="status">Memuat pengguna…</p>
    <div v-else class="mt-4 overflow-x-auto rounded-lg border">
      <Table><TableHeader><TableRow><TableHead>Nama</TableHead><TableHead>Email</TableHead><TableHead>Peran</TableHead><TableHead>Status</TableHead><TableHead class="text-right">Tindakan</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="user in users" :key="user.id"><TableCell class="font-medium">{{ user.full_name }}</TableCell><TableCell>{{ user.email }}</TableCell><TableCell>{{ roleLabels[user.role] }}</TableCell><TableCell><Badge variant="outline">{{ user.is_active ? 'Aktif' : 'Nonaktif' }}</Badge></TableCell><TableCell class="text-right"><Button variant="ghost" size="sm" :disabled="user.id === auth.session.value?.user.id" @click="edit(user)">{{ user.id === auth.session.value?.user.id ? 'Akun Anda' : 'Edit' }}</Button></TableCell></TableRow><TableRow v-if="!users.length"><TableCell colspan="5" class="py-8 text-center text-muted-foreground">Tidak ada pengguna sesuai pencarian.</TableCell></TableRow></TableBody></Table>
    </div>
    <div class="mt-4 flex items-center justify-between gap-4 text-sm"><span>{{ total }} pengguna · halaman {{ page }}</span><div class="flex gap-2"><Button variant="outline" size="sm" :disabled="page <= 1 || loading" @click="changePage(-1)">Sebelumnya</Button><Button variant="outline" size="sm" :disabled="page * 20 >= total || loading" @click="changePage(1)">Berikutnya</Button></div></div>
  </div>
</template>
