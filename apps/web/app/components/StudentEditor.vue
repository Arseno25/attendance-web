<script setup lang="ts">
import { toast } from 'vue-sonner'
import { studentSchema, newStudentSchema } from '~/lib/validation/master'
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
const { errors, validate, clear } = useFormValidation(
  () => (props.student ? studentSchema : newStudentSchema),
  () => form,
)
watch(open, async (value) => {
  if (!value) return
  clear()
  message.value = ''
  Object.assign(
    form,
    { nis: '', nisn: '', full_name: '', status: 'active', class_id: '' },
    props.student ?? {},
  )
  form.nisn = props.student?.nisn ?? ''
  if (!props.student) {
    try {
      classes.value = await options<SchoolClass>('/classes', { active: 'true' })
    } catch (error) {
      message.value = apiMessage(error)
    }
  }
})
async function save() {
  if (saving.value || !validate()) return
  saving.value = true
  message.value = ''
  try {
    const body = {
      nis: form.nis,
      nisn: form.nisn,
      full_name: form.full_name,
      status: form.status,
      ...(!props.student ? { class_id: form.class_id } : {}),
    }
    await request(props.student ? `/students/${props.student.id}` : '/students', {
      method: props.student ? 'PATCH' : 'POST',
      body,
    })
    open.value = false
    emit('saved')
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
</script>
<template>
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
        <DialogTitle>{{ student ? 'Edit siswa' : 'Tambah siswa' }}</DialogTitle>
        <DialogDescription>
          {{
            student
              ? 'Mengubah status ke nonaktif, lulus, atau pindah sekolah menutup penempatan saat ini.'
              : 'Daftarkan siswa dan tempatkan pada kelas di tahun ajaran aktif.'
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
          <Field :data-invalid="!!errors.full_name">
            <FieldLabel for="student-name">Nama lengkap</FieldLabel>
            <Input
              id="student-name"
              v-model="form.full_name"
              :aria-invalid="!!errors.full_name"
              :aria-describedby="errors.full_name ? 'student-name-error' : undefined"
              required
              maxlength="120"
              :disabled="saving"
            />
            <FieldError
              v-if="errors.full_name"
              id="student-name-error"
            >
              {{ errors.full_name }}
            </FieldError>
          </Field>
          <Field :data-invalid="!!errors.nis">
            <FieldLabel for="student-nis">NIS</FieldLabel>
            <Input
              id="student-nis"
              v-model="form.nis"
              :aria-invalid="!!errors.nis"
              :aria-describedby="errors.nis ? 'student-nis-error' : undefined"
              required
              maxlength="30"
              :disabled="saving"
            />
            <FieldDescription>Nomor induk unik di sekolah ini.</FieldDescription>
            <FieldError
              v-if="errors.nis"
              id="student-nis-error"
            >
              {{ errors.nis }}
            </FieldError>
          </Field>
          <Field :data-invalid="!!errors.nisn">
            <FieldLabel for="student-nisn">NISN (opsional)</FieldLabel>
            <Input
              id="student-nisn"
              v-model="form.nisn"
              :aria-invalid="!!errors.nisn"
              :aria-describedby="errors.nisn ? 'student-nisn-error' : undefined"
              maxlength="32"
              :disabled="saving"
            />
            <FieldError
              v-if="errors.nisn"
              id="student-nisn-error"
            >
              {{ errors.nisn }}
            </FieldError>
          </Field>
          <Field
            v-if="!student"
            :data-invalid="!!errors.class_id"
          >
            <FieldLabel for="student-class">Kelas awal</FieldLabel>
            <Select
              v-model="form.class_id"
              :disabled="saving"
            >
              <SelectTrigger
                id="student-class"
                :aria-invalid="!!errors.class_id"
                :aria-describedby="errors.class_id ? 'student-class-error' : undefined"
                class="w-full"
              >
                <SelectValue placeholder="Pilih kelas" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="item in classes"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.name }} · {{ item.academic_year_name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <FieldDescription v-if="!classes.length">
              Aktifkan tahun ajaran dan tambahkan kelas terlebih dahulu.
            </FieldDescription>
            <FieldError
              v-if="errors.class_id"
              id="student-class-error"
            >
              {{ errors.class_id }}
            </FieldError>
          </Field>
          <Field
            v-if="student"
            :data-invalid="!!errors.status"
          >
            <FieldLabel for="student-status">Status</FieldLabel>
            <Select
              v-model="form.status"
              :disabled="saving"
            >
              <SelectTrigger
                id="student-status"
                :aria-invalid="!!errors.status"
                :aria-describedby="errors.status ? 'student-status-error' : undefined"
                class="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="(label, value) in statusLabels"
                  :key="value"
                  :value="value"
                >
                  {{ label }}
                </SelectItem>
              </SelectContent>
            </Select>
            <FieldDescription>
              Setelah mengaktifkan kembali siswa, gunakan Penempatan kelas untuk memulai periode
              baru.
            </FieldDescription>
            <FieldError
              v-if="errors.status"
              id="student-status-error"
            >
              {{ errors.status }}
            </FieldError>
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
            Simpan siswa
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
