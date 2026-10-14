<script setup lang="ts">
import { toast } from 'vue-sonner'
import { placementSchema } from '~/lib/validation/master'
import { statusVariant } from '~/lib/status'
import { ArrowLeft, ArrowRightLeft, Pencil } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { statusLabels, type Student, type SchoolClass } from '~/types/master'
definePageMeta({ layout: 'admin', middleware: ['auth', 'academics'] })
const { request, options, canWrite, dateTime } = useMasterData()
const route = useRoute()
const student = ref<Student | null>(null)
const message = ref('')
const editOpen = ref(false)
const placementOpen = ref(false)
const saving = ref(false)
const classes = ref<SchoolClass[]>([])
const form = reactive({ class_id: '', reason: '' })
const { errors, validate, clear } = useFormValidation(
  () => placementSchema,
  () => form,
)
async function load() {
  message.value = ''
  try {
    student.value = (await request<Result<Student>>(`/students/${route.params.id}`)).data
  } catch (error) {
    student.value = null
    message.value = apiMessage(error)
  }
}
async function place() {
  clear()
  message.value = ''

  form.class_id = ''
  form.reason = ''
  try {
    classes.value = await options<SchoolClass>('/classes', { active: 'true' })
    placementOpen.value = true
  } catch (error) {
    message.value = apiMessage(error)
  }
}
async function savePlacement() {
  if (saving.value || !validate()) return
  saving.value = true
  message.value = ''
  try {
    await request(`/students/${route.params.id}/placements`, { method: 'POST', body: form })
    placementOpen.value = false
    toast.success('Penempatan baru tersimpan; histori sebelumnya tetap tersedia.')
    await load()
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    saving.value = false
  }
}
async function saved() {
  toast.success('Data siswa berhasil diperbarui.')
  await load()
}
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <div>
      <Button
        as-child
        variant="ghost"
        size="sm"
        class="-ml-3"
      >
        <NuxtLink to="/admin/students">
          <ArrowLeft />
          Daftar siswa
        </NuxtLink>
      </Button>
    </div>
    <Alert
      v-if="message && !placementOpen"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <template v-if="student">
      <PageHeader
        :title="student.full_name"
        :description="`NIS ${student.nis} · ${student.class_name ?? 'Belum ditempatkan'}`"
      >
        <template #actions>
          <Button
            as-child
            variant="outline"
          >
            <NuxtLink :to="{ path: '/admin/reports', query: { student_id: student.id } }">
              Lihat riwayat kehadiran
            </NuxtLink>
          </Button>
          <div
            v-if="canWrite"
            class="flex flex-wrap gap-2"
          >
            <Button
              variant="outline"
              @click="editOpen = true"
            >
              <Pencil />
              Edit siswa
            </Button>
            <Button
              v-if="student.status === 'active'"
              @click="place"
            >
              <ArrowRightLeft />
              Penempatan kelas
            </Button>
          </div>
        </template>
      </PageHeader>
      <Card>
        <CardHeader>
          <CardTitle>Identitas siswa</CardTitle>
          <CardDescription>Informasi siswa di sekolah Anda.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl class="grid gap-6 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-sm text-muted-foreground">NIS</dt>
              <dd class="mt-1 font-medium">{{ student.nis }}</dd>
            </div>
            <div>
              <dt class="text-sm text-muted-foreground">NISN</dt>
              <dd class="mt-1 font-medium">{{ student.nisn ?? 'Belum diisi' }}</dd>
            </div>
            <div>
              <dt class="text-sm text-muted-foreground">Status</dt>
              <dd class="mt-1">
                <Badge :variant="statusVariant(student.status)">
                  {{ statusLabels[student.status] }}
                </Badge>
              </dd>
            </div>
            <div>
              <dt class="text-sm text-muted-foreground">Tahun ajaran terakhir</dt>
              <dd class="mt-1 font-medium">{{ student.academic_year_name ?? '—' }}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>
      <Card class="min-w-0">
        <CardHeader>
          <CardTitle>Histori penempatan</CardTitle>
          <CardDescription>
            Waktu akhir menandai berakhirnya penempatan.
            {{
              canWrite
                ? 'Histori tidak dihapus saat siswa pindah kelas.'
                : 'Hanya histori pada kelas yang dapat Anda akses ditampilkan.'
            }}
          </CardDescription>
        </CardHeader>
        <CardContent class="min-w-0">
          <DataTable
            class="min-w-[720px]"
            :data="student.placements ?? []"
            :columns="[
              { id: 'kelas', label: 'Kelas', cellClass: 'font-medium' },
              { id: 'mulai', label: 'Mulai' },
              { id: 'berakhir', label: 'Berakhir' },
              { id: 'alasan', label: 'Alasan', cellClass: 'max-w-64 whitespace-normal' },
            ]"
            caption="Histori kelas siswa"
            row-key="id"
          >
            <template #cell-kelas="{ item }">
              {{ item.class_name }}
              <div class="text-xs font-normal text-muted-foreground">
                {{ item.academic_year_name }}
              </div>
            </template>
            <template #cell-mulai="{ item }">{{ dateTime(item.joined_at) }}</template>
            <template #cell-berakhir="{ item }">{{ dateTime(item.left_at) }}</template>
            <template #cell-alasan="{ item }">{{ item.reason }}</template>
            <template #empty>Belum ada penempatan yang dapat ditampilkan.</template>
          </DataTable>
        </CardContent>
      </Card>
      <StudentEditor
        v-if="canWrite"
        v-model:open="editOpen"
        :student="student"
        @saved="saved"
      />
    </template>
    <Dialog
      :open="placementOpen"
      @update:open="
        (value) => {
          if (!saving) placementOpen = value
        }
      "
    >
      <DialogContent
        :show-close-button="!saving"
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>Penempatan kelas</DialogTitle>
          <DialogDescription>
            Penempatan dimulai sekarang dan berakhir di akhir tahun ajaran. Periode kelas sebelumnya
            akan ditutup.
          </DialogDescription>
        </DialogHeader>
        <form
          novalidate
          class="flex flex-col gap-6"
          @submit.prevent="savePlacement"
        >
          <Alert
            v-if="message"
            variant="destructive"
          >
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <FieldGroup>
            <Field :data-invalid="!!errors.class_id">
              <FieldLabel for="placement-class">Kelas tujuan</FieldLabel>
              <Select
                v-model="form.class_id"
                :disabled="saving"
              >
                <SelectTrigger
                  id="placement-class"
                  :aria-invalid="!!errors.class_id"
                  :aria-describedby="errors.class_id ? 'placement-class-error' : undefined"
                  class="w-full"
                >
                  <SelectValue placeholder="Pilih kelas aktif" />
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
              <FieldError
                v-if="errors.class_id"
                id="placement-class-error"
              >
                {{ errors.class_id }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.reason">
              <FieldLabel for="placement-reason">Alasan penempatan</FieldLabel>
              <Textarea
                id="placement-reason"
                v-model="form.reason"
                :aria-invalid="!!errors.reason"
                :aria-describedby="errors.reason ? 'placement-reason-error' : undefined"
                required
                maxlength="500"
                :disabled="saving"
                placeholder="Contoh: perpindahan rombongan belajar"
              />
              <FieldError
                v-if="errors.reason"
                id="placement-reason-error"
              >
                {{ errors.reason }}
              </FieldError>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="saving"
              @click="placementOpen = false"
            >
              Batal
            </Button>
            <Button
              type="submit"
              :disabled="saving"
            >
              <Spinner v-if="saving" />
              Simpan penempatan
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>
