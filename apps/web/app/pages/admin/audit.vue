<script setup lang="ts">
import type { Result } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth', 'super-admin'] })
interface Entry { id: number, actor_id: string | null, action: string, target_type: string, target_id: string | null, request_id: string, occurred_at: string }
const request = useApi()
const auth = useAuth()
const entries = ref<Entry[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const message = ref('')
const labels: Record<string, string> = { 'auth.bootstrap': 'Admin pertama dibuat', 'auth.login': 'Masuk', 'auth.login_failed': 'Percobaan masuk gagal', 'auth.logout': 'Keluar', 'users.created': 'Pengguna dibuat', 'users.updated': 'Pengguna diperbarui' }
const date = (value: string) => new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short', timeZone: auth.session.value?.school.timezone ?? 'Asia/Jakarta' }).format(new Date(value))
async function load() {
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<Entry[]>>('/audit-logs', { query: { page: page.value, page_size: 20 } })
    entries.value = result.data
    total.value = result.pagination?.total ?? 0
  }
  catch (error) { message.value = apiMessage(error) }
  finally { loading.value = false }
}
async function changePage(delta: number) { page.value += delta; await load() }
await load()
</script>

<template>
  <div class="max-w-6xl">
    <div class="flex items-center justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">Audit aktivitas</h1><p class="mt-2 text-sm leading-6 text-muted-foreground">Histori perubahan akun dan sesi, tanpa password atau data biometrik.</p></div><Button variant="outline" :disabled="loading" @click="load">Muat ulang</Button></div>
    <Alert v-if="message" class="mt-6" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
    <p v-if="loading" class="mt-8 text-sm" role="status">Memuat aktivitas…</p>
    <div v-else class="mt-8 overflow-x-auto rounded-lg border"><Table><TableHeader><TableRow><TableHead>Waktu</TableHead><TableHead>Aktivitas</TableHead><TableHead>Pelaku</TableHead><TableHead>Target</TableHead><TableHead>Request ID</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="entry in entries" :key="entry.id"><TableCell class="whitespace-nowrap">{{ date(entry.occurred_at) }}</TableCell><TableCell>{{ labels[entry.action] ?? entry.action }}</TableCell><TableCell>{{ entry.actor_id?.slice(0, 8) ?? 'Anonim' }}</TableCell><TableCell>{{ entry.target_id?.slice(0, 8) ?? '—' }}</TableCell><TableCell class="font-mono text-xs">{{ entry.request_id }}</TableCell></TableRow><TableRow v-if="!entries.length"><TableCell colspan="5" class="py-8 text-center text-muted-foreground">Belum ada aktivitas tercatat.</TableCell></TableRow></TableBody></Table></div>
    <div class="mt-4 flex items-center justify-between gap-4 text-sm"><span>{{ total }} aktivitas · halaman {{ page }}</span><div class="flex gap-2"><Button variant="outline" size="sm" :disabled="page <= 1 || loading" @click="changePage(-1)">Sebelumnya</Button><Button variant="outline" size="sm" :disabled="page * 20 >= total || loading" @click="changePage(1)">Berikutnya</Button></div></div>
  </div>
</template>
