<script setup lang="ts">
const config = useRuntimeConfig()
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
        <NuxtLink to="/" class="text-base font-semibold tracking-tight">
          Absensi Sekolah
        </NuxtLink>
        <Button as-child variant="outline"><NuxtLink to="/login">Masuk</NuxtLink></Button>
      </div>
    </header>
    <main class="mx-auto max-w-5xl px-6 py-16 sm:py-24">
      <h1 class="max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
        Pencatatan kehadiran yang lebih teratur.
      </h1>
      <p class="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
        Sistem sedang disiapkan. Pengelolaan siswa, pencatatan absensi,
        dan laporan akan tersedia setelah pengaturan sekolah selesai.
      </p>
      <Card class="mt-10 max-w-xl">
        <CardHeader>
          <CardTitle>Status layanan</CardTitle>
          <CardDescription>
            Periksa koneksi sebelum melanjutkan pengaturan.
          </CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap items-center justify-between gap-4">
          <p role="status" aria-live="polite" class="text-sm">{{ serviceLabel }}</p>
          <Button variant="outline" :disabled="status === 'pending'" @click="refresh()">
            Periksa kembali
          </Button>
        </CardContent>
      </Card>
      <p class="mt-6 max-w-xl text-sm leading-6 text-muted-foreground">
        Kamera belum diaktifkan. Gunakan pencatatan sekolah yang biasa
        digunakan sampai sistem siap beroperasi.
      </p>
    </main>
  </div>
</template>
