<script setup lang="ts">
import { Save } from '@lucide/vue'
import { profileSchema } from '~/lib/validation/auth'
import { roleLabels } from '~/types/auth'

definePageMeta({ layout: 'admin', middleware: ['auth'] })
useHead({ title: 'Akun saya' })

const auth = useAuth()
const form = reactive({
  full_name: '',
  email: '',
  current_password: '',
  new_password: '',
  confirm_password: '',
})
const busy = ref(false)
const error = ref('')
const success = ref('')
const { errors, validate, clear } = useFormValidation(
  () => profileSchema(auth.session.value?.user.email ?? ''),
  () => form,
)
const sensitiveChange = computed(
  () => form.email.trim().toLowerCase() !== auth.session.value?.user.email || !!form.new_password,
)

function reset() {
  Object.assign(form, {
    full_name: auth.session.value?.user.full_name ?? '',
    email: auth.session.value?.user.email ?? '',
    current_password: '',
    new_password: '',
    confirm_password: '',
  })
  clear()
}

async function save() {
  if (busy.value || !validate()) return
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    await auth.saveProfile({
      full_name: form.full_name.trim(),
      email: form.email.trim().toLowerCase(),
      ...(sensitiveChange.value ? { current_password: form.current_password } : {}),
      ...(form.new_password ? { new_password: form.new_password } : {}),
    })
    reset()
    success.value = 'Akun berhasil diperbarui.'
  } catch (cause) {
    error.value = apiMessage(cause)
  } finally {
    busy.value = false
  }
}

reset()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Akun saya"
      description="Perbarui identitas dan password akun yang sedang Anda gunakan."
    >
      <template #actions>
        <Button
          type="submit"
          form="account-form"
          :disabled="busy"
        >
          <Spinner v-if="busy" />
          <Save v-else />
          Simpan perubahan
        </Button>
      </template>
    </PageHeader>
    <Alert
      v-if="error"
      variant="destructive"
      role="alert"
    >
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <Alert
      v-if="success"
      role="status"
    >
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>
    <form
      id="account-form"
      novalidate
      class="grid items-start gap-6 xl:grid-cols-2"
      @submit.prevent="save"
    >
      <Card>
        <CardHeader>
          <CardTitle>Profil</CardTitle>
          <CardDescription>Peran dan izin dikelola administrator sekolah.</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field :data-invalid="!!errors.full_name">
              <FieldLabel for="account-name">Nama lengkap</FieldLabel>
              <Input
                id="account-name"
                v-model="form.full_name"
                autocomplete="name"
                :disabled="busy"
                :aria-invalid="!!errors.full_name"
                :aria-describedby="errors.full_name ? 'account-name-error' : undefined"
              />
              <FieldError
                v-if="errors.full_name"
                id="account-name-error"
              >
                {{ errors.full_name }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.email">
              <FieldLabel for="account-email">Email</FieldLabel>
              <Input
                id="account-email"
                v-model="form.email"
                type="email"
                autocomplete="username"
                :disabled="busy"
                :aria-invalid="!!errors.email"
                :aria-describedby="errors.email ? 'account-email-error' : undefined"
              />
              <FieldError
                v-if="errors.email"
                id="account-email-error"
              >
                {{ errors.email }}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel>Peran</FieldLabel>
              <div>
                <Badge variant="secondary">
                  {{ auth.session.value ? roleLabels[auth.session.value.user.role] : '—' }}
                </Badge>
              </div>
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Keamanan akun</CardTitle>
          <CardDescription>
            Isi password baru jika ingin menggantinya. Perubahan email atau password mengakhiri sesi
            lain.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field :data-invalid="!!errors.new_password">
              <FieldLabel for="account-new-password">Password baru</FieldLabel>
              <Input
                id="account-new-password"
                v-model="form.new_password"
                type="password"
                autocomplete="new-password"
                minlength="8"
                :disabled="busy"
                :aria-invalid="!!errors.new_password"
                :aria-describedby="
                  errors.new_password
                    ? 'account-password-help account-new-password-error'
                    : 'account-password-help'
                "
              />
              <PasswordRequirements
                id="account-password-help"
                :value="form.new_password"
              />
              <FieldError
                v-if="errors.new_password"
                id="account-new-password-error"
              >
                {{ errors.new_password }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.confirm_password">
              <FieldLabel for="account-confirm-password">Konfirmasi password baru</FieldLabel>
              <Input
                id="account-confirm-password"
                v-model="form.confirm_password"
                type="password"
                autocomplete="new-password"
                :disabled="busy"
                :aria-invalid="!!errors.confirm_password"
                :aria-describedby="
                  errors.confirm_password
                    ? 'account-confirm-password-error'
                    : 'account-confirm-password-help'
                "
              />
              <FieldDescription id="account-confirm-password-help">
                Masukkan ulang password baru yang sama persis.
              </FieldDescription>
              <FieldError
                v-if="errors.confirm_password"
                id="account-confirm-password-error"
              >
                {{ errors.confirm_password }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.current_password">
              <FieldLabel for="account-current-password">Password saat ini</FieldLabel>
              <Input
                id="account-current-password"
                v-model="form.current_password"
                type="password"
                autocomplete="current-password"
                :disabled="busy"
                :aria-invalid="!!errors.current_password"
                :aria-describedby="
                  errors.current_password
                    ? 'account-current-password-error'
                    : 'account-current-help'
                "
              />
              <FieldDescription id="account-current-help">
                {{
                  sensitiveChange
                    ? 'Wajib untuk perubahan email atau password.'
                    : 'Diperlukan saat mengubah email atau password.'
                }}
              </FieldDescription>
              <FieldError
                v-if="errors.current_password"
                id="account-current-password-error"
              >
                {{ errors.current_password }}
              </FieldError>
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>
    </form>
  </div>
</template>
