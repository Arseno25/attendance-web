<script setup lang="ts">
import type { Result } from '~/types/auth'
const props = defineProps<{ studentId: string }>()
const { request, options, dateTime } = useMasterData()
interface Guardian { id: string, full_name: string, relationship: string }
interface Consent { id: string, guardian_name: string, policy_version: string, evidence_ref: string, valid_until: string, withdrawn_at: string | null }
const guardians = ref<Guardian[]>([])
const consents = ref<Consent[]>([])
const error = ref(''); const success = ref(''); const busy = ref(false)
const guardianOpen = ref(false); const consentOpen = ref(false); const withdrawing = ref<Consent | null>(null)
const guardian = reactive({ full_name: '', relationship: 'parent', verification_ref: '' })
const consent = reactive({ guardian_id: '', policy_version: '', evidence_ref: '', valid_until: '' })
const reason = ref('')
async function load() {
  try {
    const data = await Promise.all([options<Guardian>(`/students/${props.studentId}/guardians`), options<Consent>(`/students/${props.studentId}/consents`)])
    guardians.value = data[0]; consents.value = data[1]
  }
  catch (e) { error.value = apiMessage(e) }
}
async function save(kind: 'guardian' | 'consent' | 'withdraw') {
  busy.value = true; error.value = ''; success.value = ''
  try {
    const path = kind === 'withdraw' ? `/consents/${withdrawing.value?.id}/withdraw` : `/students/${props.studentId}/${kind === 'guardian' ? 'guardians' : 'consents'}`
    const body = kind === 'guardian' ? guardian : kind === 'consent' ? { ...consent, valid_until: new Date(consent.valid_until).toISOString() } : { reason: reason.value }
    await request<Result<unknown>>(path, { method: 'POST', body })
    guardianOpen.value = false; consentOpen.value = false; withdrawing.value = null
    success.value = kind === 'withdraw' ? 'Consent dicabut. Siswa tidak eligible untuk biometrik.' : 'Data tersimpan.'
    await load()
  }
  catch (e) { error.value = apiMessage(e) }
  finally { busy.value = false }
}
await load()
</script>
<template>
  <Card class="min-w-0"><CardHeader class="has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]"><CardTitle>Wali & persetujuan biometrik</CardTitle><CardDescription>Petugas mencatat hasil verifikasi dan referensi bukti yang disimpan melalui prosedur sekolah. Persetujuan tidak mengaktifkan recognition otomatis.</CardDescription><CardAction class="col-start-1 row-start-3 row-span-1 sm:col-start-2 sm:row-start-1 sm:row-span-2"><div class="flex flex-wrap gap-2"><Button variant="outline" size="sm" @click="Object.assign(guardian, { full_name: '', relationship: 'parent', verification_ref: '' }); error = ''; guardianOpen = true">Tambah wali</Button><Button size="sm" :disabled="!guardians.length" @click="Object.assign(consent, { guardian_id: '', policy_version: '', evidence_ref: '', valid_until: '' }); error = ''; consentOpen = true">Catat consent</Button></div></CardAction></CardHeader><CardContent class="space-y-4">
    <Alert v-if="error && !guardianOpen && !consentOpen && !withdrawing" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><Alert v-if="success" role="status"><AlertDescription>{{ success }}</AlertDescription></Alert>
    <Table><TableCaption class="sr-only">Riwayat persetujuan biometrik</TableCaption><TableHeader><TableRow><TableHead>Wali</TableHead><TableHead>Kebijakan</TableHead><TableHead>Berlaku sampai</TableHead><TableHead>Status</TableHead><TableHead><span class="sr-only">Aksi</span></TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in consents" :key="item.id"><TableCell>{{ item.guardian_name }}</TableCell><TableCell>{{ item.policy_version }}</TableCell><TableCell>{{ dateTime(item.valid_until) }}</TableCell><TableCell><Badge variant="outline">{{ item.withdrawn_at ? 'Dicabut' : new Date(item.valid_until) <= new Date() ? 'Kedaluwarsa' : 'Berlaku' }}</Badge></TableCell><TableCell><Button v-if="!item.withdrawn_at" variant="outline" size="sm" @click="withdrawing = item; reason = ''; error = ''">Cabut</Button></TableCell></TableRow><TableRow v-if="!consents.length"><TableCell colspan="5" class="py-8 text-center text-muted-foreground">Belum ada consent. Absensi manual tetap tersedia.</TableCell></TableRow></TableBody></Table>
  </CardContent></Card>
  <Sheet :open="guardianOpen" @update:open="v => { if (!busy) guardianOpen = v }"><SheetContent class="overflow-y-auto"><SheetHeader><SheetTitle>Verifikasi wali</SheetTitle><SheetDescription>Isi setelah identitas dan hubungan wali diverifikasi sesuai prosedur sekolah.</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="save('guardian')"><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="guardian-name">Nama wali</FieldLabel><Input id="guardian-name" v-model="guardian.full_name" required maxlength="120" :disabled="busy" /></Field><Field><FieldLabel for="guardian-relation">Hubungan</FieldLabel><Select v-model="guardian.relationship" :disabled="busy"><SelectTrigger id="guardian-relation" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="parent">Orang tua</SelectItem><SelectItem value="guardian">Wali sah</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="guardian-ref">Referensi verifikasi</FieldLabel><Input id="guardian-ref" v-model="guardian.verification_ref" required maxlength="120" :disabled="busy" /><FieldDescription>Nomor referensi internal; tidak perlu mengunggah dokumen identitas.</FieldDescription></Field></FieldGroup><Button type="submit" :disabled="busy" class="w-full"><Spinner v-if="busy" />Simpan hasil verifikasi</Button></form></SheetContent></Sheet>
  <Sheet :open="consentOpen" @update:open="v => { if (!busy) consentOpen = v }"><SheetContent class="overflow-y-auto"><SheetHeader><SheetTitle>Catat persetujuan</SheetTitle><SheetDescription>Persetujuan berlaku hingga waktu yang ditetapkan kebijakan sekolah. Absensi manual tetap tersedia jika tidak bersedia.</SheetDescription></SheetHeader><form class="space-y-6 px-4 pb-6" @submit.prevent="save('consent')"><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><FieldGroup><Field><FieldLabel for="consent-guardian">Wali terverifikasi</FieldLabel><Select v-model="consent.guardian_id" :disabled="busy"><SelectTrigger id="consent-guardian" class="w-full"><SelectValue placeholder="Pilih wali" /></SelectTrigger><SelectContent><SelectItem v-for="item in guardians" :key="item.id" :value="item.id">{{ item.full_name }}</SelectItem></SelectContent></Select></Field><Field><FieldLabel for="policy">Versi kebijakan</FieldLabel><Input id="policy" v-model="consent.policy_version" required maxlength="80" :disabled="busy" /></Field><Field><FieldLabel for="evidence">Referensi bukti persetujuan</FieldLabel><Input id="evidence" v-model="consent.evidence_ref" required maxlength="120" :disabled="busy" /></Field><Field><FieldLabel for="valid-until">Berlaku sampai (waktu perangkat)</FieldLabel><Input id="valid-until" v-model="consent.valid_until" type="datetime-local" required :disabled="busy" /></Field></FieldGroup><Button type="submit" :disabled="busy || !consent.guardian_id" class="w-full"><Spinner v-if="busy" />Simpan consent</Button></form></SheetContent></Sheet>
  <Dialog :open="!!withdrawing" @update:open="v => { if (!v && !busy) withdrawing = null }"><DialogContent><DialogHeader><DialogTitle>Cabut persetujuan</DialogTitle><DialogDescription>Pencabutan langsung menghilangkan eligibility biometrik. Riwayat absensi tetap tersimpan.</DialogDescription></DialogHeader><form class="space-y-4" @submit.prevent="save('withdraw')"><Alert v-if="error" variant="destructive"><AlertDescription>{{ error }}</AlertDescription></Alert><Field><FieldLabel for="withdraw-reason">Alasan</FieldLabel><Textarea id="withdraw-reason" v-model="reason" required maxlength="500" :disabled="busy" /></Field><Button type="submit" variant="destructive" :disabled="busy"><Spinner v-if="busy" />Cabut consent</Button></form></DialogContent></Dialog>
</template>
