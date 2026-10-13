<script setup lang="ts">
import type { Student, SchoolClass } from '~/types/master'
import { statusLabels } from '~/types/master'
const props = defineProps<{ student?: Student }>()
const emit = defineEmits<{ saved: [] }>()
const open = defineModel<boolean>('open', { required: true })
const { request, options } = useMasterData()
const classes = ref<SchoolClass[]>([])
const saving = ref(false)
const message = ref('')
const form = reactive({ nis: '', nisn: '', full_name: '', status: 'active', class_id: '' })
watch(open, async (value) => {
  if (!value) return
  message.value = ''
  Object.assign(form, { nis: '', nisn: '', full_name: '', status: 'active', class_id: '' }, props.student ?? {})
  form.nisn = props.student?.nisn ?? ''
  if (!props.student) {
    try { classes.value = await options<SchoolClass>('/classes', { active: 'true' }) }
    catch (error) { message.value = apiMessage(error) }
  }
})
async function save() {
  saving.value = true; message.value = ''
  try {
    const body = { nis: form.nis, nisn: form.nisn, full_name: form.full_name, status: form.status, ...(!props.student ? { class_id: form.class_id } : {}) }
    await request(props.student ? `/students/${props.student.id}` : '/students', { method: props.student ? 'PATCH' : 'POST', body })
    open.value = false; emit('saved')
  }
  catch (error) { message.value = apiMessage(error) }
  finally { saving.value = false }
}
</script>
<template>
  <Sheet :open="open" @update:open="value => { if (!saving) open = value }"><SheetContent class="overflow-y-auto sm:max-w-lg"><SheetHeader><SheetTitle>{{ student ? 'Edit siswa' : 'Tambah siswa' }}</SheetTitle><SheetDescription>{{ student ? 'Mengubah status ke nonaktif, lulus, atau pindah sekolah menutup penempatan saat ini.' : 'Daftarkan siswa dan tempatkan pada kelas di tahun ajaran aktif.' }}</SheetDescription></SheetHeader>
    <form class="space-y-6 px-4 pb-6" @submit.prevent="save"><Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert><FieldGroup>
      <Field><FieldLabel for="student-name">Nama lengkap</FieldLabel><Input id="student-name" v-model="form.full_name" required maxlength="120" :disabled="saving" /></Field>
      <Field><FieldLabel for="student-nis">NIS</FieldLabel><Input id="student-nis" v-model="form.nis" required maxlength="30" :disabled="saving" /><FieldDescription>Nomor induk unik di sekolah ini.</FieldDescription></Field>
      <Field><FieldLabel for="student-nisn">NISN (opsional)</FieldLabel><Input id="student-nisn" v-model="form.nisn" maxlength="32" :disabled="saving" /></Field>
      <Field v-if="!student"><FieldLabel for="student-class">Kelas awal</FieldLabel><Select v-model="form.class_id" :disabled="saving"><SelectTrigger id="student-class" class="w-full"><SelectValue placeholder="Pilih kelas" /></SelectTrigger><SelectContent><SelectItem v-for="item in classes" :key="item.id" :value="item.id">{{ item.name }} · {{ item.academic_year_name }}</SelectItem></SelectContent></Select><FieldDescription v-if="!classes.length">Aktifkan tahun ajaran dan tambahkan kelas terlebih dahulu.</FieldDescription></Field>
      <Field v-if="student"><FieldLabel for="student-status">Status</FieldLabel><Select v-model="form.status" :disabled="saving"><SelectTrigger id="student-status" class="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem v-for="(label, value) in statusLabels" :key="value" :value="value">{{ label }}</SelectItem></SelectContent></Select><FieldDescription>Setelah mengaktifkan kembali siswa, gunakan Penempatan kelas untuk memulai periode baru.</FieldDescription></Field>
    </FieldGroup><Button class="w-full" type="submit" :disabled="saving || (!student && !form.class_id)"><Spinner v-if="saving" />Simpan siswa</Button></form>
  </SheetContent></Sheet>
</template>
