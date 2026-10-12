<script setup lang="ts">
definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
</script>

<template>
  <div class="max-w-4xl">
    <h1 class="text-2xl font-semibold tracking-tight">Ringkasan</h1>
    <p class="mt-2 text-muted-foreground">{{ auth.session.value?.school.name }}</p>
    <Alert v-if="route.query.forbidden" class="mt-6" role="alert"><AlertDescription>Halaman tersebut hanya tersedia untuk pengguna yang memiliki izin.</AlertDescription></Alert>
    <Card class="mt-8">
      <CardHeader><CardTitle>Akses sekolah sudah siap</CardTitle><CardDescription>Anda masuk sebagai {{ auth.session.value?.user.full_name }}.</CardDescription></CardHeader>
      <CardContent class="space-y-4 text-sm leading-6">
        <p>Pengaturan akun, peran, dan audit aktivitas sudah tersedia. Data siswa serta pencatatan absensi akan ditambahkan pada tahap berikutnya.</p>
        <Button v-if="auth.session.value?.user.role === 'super_admin'" as-child><NuxtLink to="/admin/users">Kelola pengguna</NuxtLink></Button>
        <p v-if="auth.session.value?.user.role === 'teacher'" class="text-muted-foreground">Akses data kelas akan tersedia setelah penugasan guru dikonfigurasi.</p>
      </CardContent>
    </Card>
  </div>
</template>
