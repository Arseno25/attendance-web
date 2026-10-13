<script setup lang="ts">
import type { Result } from '~/types/auth'
definePageMeta({ layout: 'admin', middleware: ['auth', 'devices'] })
interface Device { id: string, name: string, location: string, revoked_at: string | null, last_seen_at: string | null }
const { request, dateTime } = useMasterData()
const auth = useAuth()
const canManage = computed(() => auth.session.value?.permissions.includes('devices.manage'))
const devices = ref<Device[]>([]); const page = ref(1); const total = ref(0)
const loading = ref(false); const busy = ref(false); const error = ref(''); const success = ref('')
const editor = ref(false); const revoking = ref<Device | null>(null); const reason = ref('')
const pairing = ref<{ pairing_token: string, expires_at: string } | null>(null)
const form = reactive({ name: '', location: '' })
async function load() {
  loading.value = true; error.value = ''
  try { const result = await request<Result<Device[]>>('/devices', { query: { page: page.value, page_size: 20 } }); devices.value = result.data; total.value = result.pagination?.total ?? 0 }
  catch (e) { devices.value = []; error.value = apiMessage(e) }
  finally { loading.value = false }
}
async function act(kind: 'create' | 'revoke' | 'pair', id?: string) {
  busy.value = true; error.value = ''; success.value = ''
  try {
    if (kind === 'pair') pairing.value = (await request<Result<{ pairing_token: string, expires_at: string }>>(`/devices/${id}/pairing-token`, { method: 'POST' })).data
    else { await request(kind === 'create' ? '/devices' : `/devices/${revoking.value?.id}/revoke`, { method: 'POST', body: kind === 'create' ? form : { reason: reason.value } }); editor.value = false; revoking.value = null; success.value = kind === 'create' ? 'Perangkat didaftarkan.' : 'Perangkat dicabut; sesi kiosk langsung tidak berlaku.'; await load() }
  }
  catch (e) { error.value = apiMessage(e) }
  finally { busy.value = false }
}
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6"><div class="flex flex-wrap items-start justify-between gap-4"><div><h1 class="text-2xl font-semibold tracking-tight">Perangkat kiosk</h1><p class="mt-2 text-sm text-muted-foreground">Daftarkan perangkat, pasangkan pada sesi dan pantau heartbeat.</p></div><div class="flex gap-2"><Button as-child variant="outline"><NuxtLink to="/absensi" target="_blank">Buka halaman absensi</NuxtLink></Button><Button v-if="canManage" @click="Object.assign(form, { name: '', location: '' }); error = ''; editor = true">Tambah perangkat</Button></div></div>
    <Alert v-if="error && !editor && !revoking" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Card class="min-w-0"><CardHeader><CardTitle>Perangkat terdaftar</CardTitle><CardDescription>Token pairing hanya berlaku sekali selama 5 menit. Sesi kiosk berlaku 10 menit dan harus dipasangkan ulang oleh operator.</CardDescription></CardHeader><CardContent><Table class="min-w-[600px]"><TableCaption class="sr-only">Perangkat dan heartbeat terakhir</TableCaption><TableHeader><TableRow><TableHead>Perangkat</TableHead><TableHead>Lokasi</TableHead><TableHead>Status</TableHead><TableHead>Heartbeat terakhir</TableHead><TableHead><span class="sr-only">Aksi</span></TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in devices" :key="item.id"><TableCell class="font-medium">{{ item.name }}</TableCell><TableCell>{{ item.location }}</TableCell><TableCell><Badge variant="outline">{{ item.revoked_at ? 'Dicabut' : 'Terdaftar' }}</Badge></TableCell><TableCell>{{ item.last_seen_at ? dateTime(item.last_seen_at) : 'Belum terhubung' }}</TableCell><TableCell><div v-if="canManage && !item.revoked_at" class="flex gap-2"><Button variant="outline" size="sm" :disabled="busy" @click="act('pair', item.id)">Token pairing</Button><Button variant="ghost" size="sm" @click="revoking = item; reason = ''; error = ''">Cabut</Button></div></TableCell></TableRow><TableRow v-if="!devices.length"><TableCell colspan="5" class="py-8 text-center text-muted-foreground">{{ loading ? 'Memuat perangkat…' : 'Belum ada perangkat.' }}</TableCell></TableRow></TableBody></Table><div class="mt-4 flex items-center justify-between gap-3"><p class="text-sm text-muted-foreground">{{ total }} perangkat · halaman {{ page }}</p><div class="flex gap-2"><Button variant="outline" size="sm" :disabled="loading || page === 1" @click="page--; load()">Sebelumnya</Button><Button variant="outline" size="sm" :disabled="loading || page * 20 >= total" @click="page++; load()">Berikutnya</Button></div></div></CardContent></Card>
    <Sheet :open="editor" @update:open="v => { if (!busy) editor = v }"><SheetContent><SheetHeader><SheetTitle>Daftar perangkat</SheetTitle><SheetDescription>Perangkat baru memerlukan pairing sebelum digunakan.</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="act('create')"><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="device-name">Nama perangkat</FieldLabel><Input id="device-name" v-model="form.name" required maxlength="120" :disabled="busy" /></Field><Field><FieldLabel for="device-location">Lokasi</FieldLabel><Input id="device-location" v-model="form.location" required maxlength="120" :disabled="busy" /></Field></FieldGroup><Button type="submit" :disabled="busy" class="w-full"><Spinner v-if="busy" />Daftarkan</Button></form></SheetContent></Sheet>
    <Dialog :open="!!revoking" @update:open="v => { if (!v && !busy) revoking = null }"><DialogContent><DialogHeader><DialogTitle>Cabut perangkat</DialogTitle><DialogDescription>Perangkat {{ revoking?.name }} dan semua sesi pairing akan langsung dinonaktifkan.</DialogDescription></DialogHeader><form class="space-y-4" @submit.prevent="act('revoke')"><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><Field><FieldLabel for="device-reason">Alasan pencabutan</FieldLabel><Textarea id="device-reason" v-model="reason" required maxlength="500" :disabled="busy" /></Field><Button type="submit" variant="destructive" :disabled="busy">Cabut perangkat</Button></form></DialogContent></Dialog>
    <Dialog :open="!!pairing" @update:open="v => { if (!v) pairing = null }"><DialogContent><DialogHeader><DialogTitle>Token pairing sekali pakai</DialogTitle><DialogDescription>Masukkan token ini di halaman absensi pada perangkat tujuan. Token tidak disimpan di browser.</DialogDescription></DialogHeader><Field><FieldLabel for="pairing-token">Token pairing</FieldLabel><Input id="pairing-token" :model-value="pairing?.pairing_token" readonly autocomplete="off" class="font-mono text-xs" /></Field><p class="text-sm text-muted-foreground">Berlaku sampai {{ pairing ? dateTime(pairing.expires_at) : '' }}.</p><Button @click="pairing = null">Selesai</Button></DialogContent></Dialog>
  </div>
</template>
