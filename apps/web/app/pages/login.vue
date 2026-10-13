<script setup lang="ts">
import { loginSchema } from '~/lib/validation/auth'
const auth = useAuth()
const route = useRoute()
const email = ref('')
const password = ref('')
const { errors, validate, clear } = useFormValidation(
  () => loginSchema,
  () => ({ email: email.value, password: password.value }),
)
const busy = ref(false)
const message = ref(
  route.query.expired
    ? 'Sesi berakhir. Silakan masuk kembali.'
    : route.query.unavailable
      ? 'Layanan belum tersedia. Coba kembali sebentar lagi.'
      : '',
)

async function submit() {
  if (busy.value || !validate()) return
  busy.value = true
  message.value = ''
  try {
    await auth.login(email.value, password.value)
    password.value = ''
    clear()
    await navigateTo('/admin')
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="relative grid min-h-screen lg:grid-cols-2">
    <ModeToggle class="absolute right-4 top-4" />
    <div class="hidden flex-col justify-between bg-muted p-12 lg:flex">
      <NuxtLink
        to="/"
        class="text-xl font-semibold tracking-tight"
      >
        Absensi Sekolah
      </NuxtLink>
      <div class="max-w-md">
        <h1 class="text-4xl font-semibold leading-tight tracking-tight">
          Satu tempat untuk mengelola kehadiran sekolah.
        </h1>
        <p class="mt-5 text-base leading-7 text-muted-foreground">
          Akses sesuai peran, pencatatan yang dapat ditelusuri, dan jalur absensi manual untuk
          setiap siswa.
        </p>
      </div>
      <p class="text-sm text-muted-foreground">Untuk petugas sekolah yang memiliki akun.</p>
    </div>
    <div class="flex items-center justify-center px-6 py-14">
      <div class="w-full max-w-sm">
        <NuxtLink
          to="/"
          class="mb-10 block text-lg font-semibold lg:hidden"
        >
          Absensi Sekolah
        </NuxtLink>
        <h2 class="text-2xl font-semibold tracking-tight">Masuk ke akun Anda</h2>
        <p class="mb-8 mt-2 text-sm leading-6 text-muted-foreground">
          Gunakan email dan password yang diberikan pengelola sekolah.
        </p>
        <Alert
          v-if="message"
          class="mb-6"
          role="alert"
        >
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
        <form
          novalidate
          class="space-y-5"
          @submit.prevent="submit"
        >
          <Field :data-invalid="!!errors.email">
            <FieldLabel for="email">Email</FieldLabel>
            <Input
              id="email"
              v-model="email"
              :aria-invalid="!!errors.email"
              :aria-describedby="errors.email ? 'email-error' : undefined"
              type="email"
              autocomplete="username"
              required
              maxlength="254"
              :disabled="busy"
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
              :aria-invalid="!!errors.password"
              :aria-describedby="errors.password ? 'password-error' : undefined"
              type="password"
              autocomplete="current-password"
              required
              maxlength="128"
              :disabled="busy"
            />
            <FieldError
              v-if="errors.password"
              id="password-error"
            >
              {{ errors.password }}
            </FieldError>
          </Field>
          <Button
            class="w-full"
            type="submit"
            :disabled="busy"
          >
            {{ busy ? 'Sedang masuk…' : 'Masuk' }}
          </Button>
        </form>
        <p class="mt-6 text-sm leading-6 text-muted-foreground">
          Belum memiliki akun? Hubungi admin sekolah.
        </p>
      </div>
    </div>
  </main>
</template>
