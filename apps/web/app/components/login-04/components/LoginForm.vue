<script setup lang="ts">
import { loginSchema } from '~/lib/validation/auth'
const auth = useAuth()
const { branding } = useSchoolBranding()
const route = useRoute()
const email = ref('')
const password = ref('')
const { errors, validate, clear } = useFormValidation(
  () => loginSchema,
  () => ({ email: email.value, password: password.value }),
)
const busy = ref(false)
const failure = ref('')
watch([email, password], () => {
  failure.value = ''
})
const message = ref(
  route.query.expired
    ? 'Sesi berakhir. Silakan masuk kembali.'
    : route.query.unavailable
      ? 'Layanan belum tersedia. Coba kembali sebentar lagi.'
      : '',
)

async function submit() {
  if (busy.value) return
  failure.value = ''
  if (!validate()) return
  busy.value = true
  message.value = ''
  try {
    await auth.login(email.value, password.value)
    password.value = ''
    clear()
    await navigateTo('/admin')
  } catch (error) {
    failure.value = apiMessage(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <Card class="overflow-hidden p-0">
      <CardContent class="grid p-0 md:grid-cols-2">
        <form
          novalidate
          aria-labelledby="login-title"
          :aria-describedby="failure ? 'login-error' : undefined"
          class="p-6 md:p-8"
          @submit.prevent="submit"
        >
          <FieldGroup>
            <div class="flex flex-col items-center gap-2 text-center">
              <h1
                id="login-title"
                class="text-2xl font-bold"
              >
                Selamat datang kembali
              </h1>
              <p class="text-balance text-muted-foreground">
                Masuk ke akun {{ branding.app_name }}.
              </p>
            </div>
            <Alert
              v-if="message"
              role="alert"
            >
              <AlertDescription>{{ message }}</AlertDescription>
            </Alert>
            <Alert
              v-if="failure"
              id="login-error"
              variant="destructive"
              role="alert"
            >
              <AlertDescription>{{ failure }}</AlertDescription>
            </Alert>
            <Field :data-invalid="!!errors.email">
              <FieldLabel for="email">Email</FieldLabel>
              <Input
                id="email"
                v-model="email"
                type="email"
                autocomplete="username"
                placeholder="nama@sekolah.sch.id"
                required
                maxlength="254"
                :disabled="busy"
                :aria-invalid="!!errors.email"
                :aria-describedby="errors.email ? 'email-error' : undefined"
              />
              <FieldError
                v-if="errors.email"
                id="email-error"
              >
                {{ errors.email }}
              </FieldError>
            </Field>
            <Field :data-invalid="!!errors.password">
              <FieldLabel for="password">Password</FieldLabel>
              <Input
                id="password"
                v-model="password"
                type="password"
                autocomplete="current-password"
                required
                :disabled="busy"
                :aria-invalid="!!errors.password"
                :aria-describedby="errors.password ? 'password-error' : undefined"
              />
              <FieldError
                v-if="errors.password"
                id="password-error"
              >
                {{ errors.password }}
              </FieldError>
            </Field>
            <Field>
              <Button
                type="submit"
                :disabled="busy"
              >
                <Spinner v-if="busy" />
                {{ busy ? 'Sedang masuk…' : 'Masuk' }}
              </Button>
            </Field>
            <FieldDescription class="text-center">
              Lupa password? Hubungi administrator sekolah.
            </FieldDescription>
          </FieldGroup>
        </form>
        <div
          class="relative hidden bg-muted md:block"
          aria-hidden="true"
        >
          <img
            src="/login-placeholder.svg"
            alt=""
            class="absolute inset-0 h-full w-full object-cover dark:brightness-[0.2] dark:grayscale"
          />
        </div>
      </CardContent>
    </Card>
    <FieldDescription class="px-6 text-center">
      Akun diterbitkan oleh administrator. Gunakan akun sesuai peran dan tugas Anda.
    </FieldDescription>
  </div>
</template>
