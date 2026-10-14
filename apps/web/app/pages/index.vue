<script setup lang="ts">
const config = useRuntimeConfig()
const { branding, load } = useSchoolBranding()
onMounted(load)
useHead(() => ({ title: branding.value.app_name }))
const { data, status, error, refresh } = await useFetch<{ status: string }>(
  `${config.public.apiBase}/health/ready`,
  { server: false, timeout: 4000, retry: 0 },
)

const serviceLabel = computed(() => {
  if (status.value === 'pending') return 'Memeriksa koneksi'
  if (error.value) return 'Layanan belum tersedia'
  return data.value?.status === 'ok' ? 'Layanan terhubung' : 'Belum diperiksa'
})
</script>

<template>
  <div>
    <header class="border-b">
      <div class="mx-auto flex max-w-5xl items-center justify-between gap-4 px-6 py-5">
        <NuxtLink
          to="/"
          class="flex min-w-0 items-center gap-2 text-base font-semibold tracking-tight"
        >
          <img
            v-if="branding.logo_url"
            :src="`${config.public.apiBase}${branding.logo_url}`"
            alt=""
            class="size-8 shrink-0 rounded-lg object-contain"
          />
          <span class="truncate">{{ branding.app_name }}</span>
        </NuxtLink>
        <Button
          as-child
          variant="outline"
        >
          <NuxtLink to="/login">Masuk</NuxtLink>
        </Button>
      </div>
    </header>
    <main class="mx-auto max-w-5xl px-6 py-16 sm:py-24">
      <h1 class="max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
        Pencatatan kehadiran yang lebih teratur.
      </h1>
      <p class="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
        Kelola data siswa, catat kehadiran, dan lihat laporan sekolah dalam satu tempat. Masuk
        dengan akun yang diberikan oleh pengelola sekolah.
      </p>
      <Card class="mt-10 max-w-xl">
        <CardHeader>
          <CardTitle>Status layanan</CardTitle>
          <CardDescription>Periksa koneksi jika halaman belum dapat dibuka.</CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap items-center justify-between gap-4">
          <p
            role="status"
            aria-live="polite"
            class="text-sm"
          >
            {{ serviceLabel }}
          </p>
          <Button
            variant="outline"
            :disabled="status === 'pending'"
            @click="refresh()"
          >
            Periksa kembali
          </Button>
        </CardContent>
      </Card>
      <p class="mt-6 max-w-xl text-sm leading-6 text-muted-foreground">
        Absensi wajah belum tersedia. Petugas tetap dapat mencatat kehadiran melalui aplikasi.
      </p>
    </main>
  </div>
</template>
