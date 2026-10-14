<script setup lang="ts">
import { toast } from 'vue-sonner'
import { Save, RefreshCw } from '@lucide/vue'
import { schoolSettingsSchema } from '~/lib/validation/settings'
import type { SchoolSettings } from '~/types/settings'
import type { Result } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth'] })
useHead({ title: 'Pengaturan' })
const request = useApi()
const auth = useAuth()
const canManageSchool = computed(
  () =>
    auth.session.value?.user.role === 'super_admin' &&
    auth.session.value.permissions.includes('school.write'),
)
const tab = ref(canManageSchool.value ? 'school' : 'appearance')
const form = reactive({ name: '', app_name: '', address: '', phone: '', email: '' })
const school = ref<SchoolSettings | null>(null)
const loading = ref(false)
const busy = ref(false)
const imageBusy = ref(false)
const failure = ref('')
const { errors, validate, clear } = useFormValidation(
  () => schoolSettingsSchema,
  () => form,
)
async function load() {
  if (loading.value || !canManageSchool.value) return
  loading.value = true
  failure.value = ''
  try {
    school.value = (await request<Result<SchoolSettings>>('/school/settings')).data
    for (const key of Object.keys(form) as (keyof typeof form)[]) form[key] = school.value[key]
    clear()
  } catch (cause) {
    failure.value = apiMessage(cause)
  } finally {
    loading.value = false
  }
}
async function imageUpdated() {
  try {
    school.value = (await request<Result<SchoolSettings>>('/school/settings')).data
    await auth.load()
  } catch {
    toast.warning('Logo disimpan. Muat ulang halaman untuk memperbarui menu.')
  }
}
async function save() {
  if (busy.value || imageBusy.value || !canManageSchool.value || !school.value || !validate())
    return
  busy.value = true
  try {
    school.value = (
      await request<Result<SchoolSettings>>('/school/settings', {
        method: 'PATCH',
        body: schoolSettingsSchema.parse(form),
      })
    ).data
    toast.success('Pengaturan sekolah disimpan.')
    try {
      await auth.load()
    } catch {
      toast.warning('Muat ulang halaman untuk menampilkan identitas sekolah terbaru.')
    }
  } catch (cause) {
    toast.error(apiMessage(cause))
  } finally {
    busy.value = false
  }
}
watch(
  tab,
  (value) => {
    if (value === 'school' && !school.value) void load()
  },
  { immediate: true },
)
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Pengaturan"
      description="Sesuaikan tampilan aplikasi dan pengaturan sekolah."
    >
      <template
        v-if="tab === 'school' && canManageSchool"
        #actions
      >
        <Button
          v-if="!school"
          variant="outline"
          :disabled="loading"
          @click="load"
        >
          <RefreshCw />
          Coba lagi
        </Button>
        <Button
          v-else
          type="submit"
          form="school-settings-form"
          :disabled="busy || loading || imageBusy"
        >
          <Spinner v-if="busy" />
          <Save v-else />
          Simpan perubahan
        </Button>
      </template>
    </PageHeader>
    <Tabs
      v-model="tab"
      class="min-w-0"
    >
      <TabsList>
        <TabsTrigger
          v-if="canManageSchool"
          value="school"
        >
          Sekolah
        </TabsTrigger>
        <TabsTrigger value="appearance">Tampilan</TabsTrigger>
      </TabsList>
      <TabsContent
        value="appearance"
        class="pt-4"
      >
        <AppearanceSettings />
      </TabsContent>
      <TabsContent
        v-if="canManageSchool"
        value="school"
        class="space-y-6 pt-4"
      >
        <Alert
          v-if="failure"
          variant="destructive"
          role="alert"
        >
          <AlertDescription>{{ failure }}</AlertDescription>
        </Alert>
        <Skeleton
          v-if="loading"
          class="h-64 w-full"
          aria-label="Memuat pengaturan sekolah"
        />
        <div
          v-else-if="school"
          class="grid min-w-0 items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(280px,360px)]"
        >
          <form
            id="school-settings-form"
            novalidate
            class="min-w-0"
            @submit.prevent="save"
          >
            <Card>
              <CardHeader>
                <CardTitle>Identitas & kontak</CardTitle>
                <CardDescription>
                  Nama dan logo sekolah tampil di halaman masuk dan menu utama.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field :data-invalid="!!errors.name">
                    <FieldLabel for="school-name">Nama sekolah</FieldLabel>
                    <Input
                      id="school-name"
                      v-model="form.name"
                      :disabled="busy"
                      :aria-invalid="!!errors.name"
                      :aria-describedby="errors.name ? 'school-name-error' : undefined"
                    />
                    <FieldError
                      v-if="errors.name"
                      id="school-name-error"
                    >
                      {{ errors.name }}
                    </FieldError>
                  </Field>
                  <Field :data-invalid="!!errors.app_name">
                    <FieldLabel for="school-app-name">Nama aplikasi</FieldLabel>
                    <Input
                      id="school-app-name"
                      v-model="form.app_name"
                      :disabled="busy"
                      :aria-invalid="!!errors.app_name"
                      :aria-describedby="errors.app_name ? 'school-app-error' : 'school-app-help'"
                    />
                    <FieldDescription id="school-app-help">
                      Contoh: Absensi Sekolah.
                    </FieldDescription>
                    <FieldError
                      v-if="errors.app_name"
                      id="school-app-error"
                    >
                      {{ errors.app_name }}
                    </FieldError>
                  </Field>
                  <Field :data-invalid="!!errors.address">
                    <FieldLabel for="school-address">
                      Alamat
                      <span class="font-normal text-muted-foreground">(opsional)</span>
                    </FieldLabel>
                    <Textarea
                      id="school-address"
                      v-model="form.address"
                      :disabled="busy"
                      :aria-invalid="!!errors.address"
                      :aria-describedby="errors.address ? 'school-address-error' : undefined"
                    />
                    <FieldError
                      v-if="errors.address"
                      id="school-address-error"
                    >
                      {{ errors.address }}
                    </FieldError>
                  </Field>
                  <div class="grid gap-6 sm:grid-cols-2">
                    <Field :data-invalid="!!errors.phone">
                      <FieldLabel for="school-phone">Telepon (opsional)</FieldLabel>
                      <Input
                        id="school-phone"
                        v-model="form.phone"
                        type="tel"
                        autocomplete="tel"
                        :disabled="busy"
                        :aria-invalid="!!errors.phone"
                        :aria-describedby="errors.phone ? 'school-phone-error' : undefined"
                      />
                      <FieldError
                        v-if="errors.phone"
                        id="school-phone-error"
                      >
                        {{ errors.phone }}
                      </FieldError>
                    </Field>
                    <Field :data-invalid="!!errors.email">
                      <FieldLabel for="school-email">Email (opsional)</FieldLabel>
                      <Input
                        id="school-email"
                        v-model="form.email"
                        type="email"
                        autocomplete="email"
                        :disabled="busy"
                        :aria-invalid="!!errors.email"
                        :aria-describedby="errors.email ? 'school-email-error' : undefined"
                      />
                      <FieldError
                        v-if="errors.email"
                        id="school-email-error"
                      >
                        {{ errors.email }}
                      </FieldError>
                    </Field>
                  </div>
                </FieldGroup>
              </CardContent>
            </Card>
          </form>
          <Card class="min-w-0">
            <CardHeader>
              <CardTitle>Logo sekolah</CardTitle>
              <CardDescription>Pilih logo sekolah yang mudah dikenali.</CardDescription>
            </CardHeader>
            <CardContent>
              <ImageUploadField
                label="Logo sekolah"
                endpoint="/school/logo"
                :value="school.logo_url"
                logo
                :disabled="busy"
                @updated="imageUpdated"
                @busy-change="imageBusy = $event"
              />
            </CardContent>
          </Card>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
