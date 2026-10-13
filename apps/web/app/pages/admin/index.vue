<script setup lang="ts">
import { ClipboardCheck, ScrollText, Users } from '@lucide/vue'
import SectionCards from '~/components/SectionCards.vue'
definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
</script>

<template>
  <div class="flex flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Ringkasan sekolah</h1><p class="mt-2 text-sm text-muted-foreground">Informasi akun dan akses pengelolaan sekolah Anda.</p></div>
    <Alert v-if="route.query.forbidden" variant="destructive" role="alert"><AlertDescription>Halaman tersebut hanya tersedia untuk pengguna yang memiliki izin.</AlertDescription></Alert>
    <SectionCards />
    <Card v-if="auth.session.value?.user.role === 'super_admin'" class="shadow-none">
      <CardHeader><CardTitle>Pengelolaan sekolah</CardTitle><CardDescription>Kelola akun petugas dan telusuri aktivitas mereka.</CardDescription></CardHeader>
      <CardContent class="flex flex-wrap gap-3">
        <Button as-child><NuxtLink to="/admin/users"><Users />Kelola pengguna</NuxtLink></Button>
        <Button as-child variant="outline"><NuxtLink to="/admin/audit"><ScrollText />Lihat audit aktivitas</NuxtLink></Button>
      </CardContent>
    </Card>
    <Empty class="border">
      <EmptyHeader><EmptyMedia variant="icon"><ClipboardCheck /></EmptyMedia><EmptyTitle>Data kehadiran belum tersedia</EmptyTitle><EmptyDescription>Data siswa dan pencatatan absensi akan tersedia pada tahap berikutnya. Halaman absensi akan dibuka terpisah dari panel pengelolaan.</EmptyDescription></EmptyHeader>
      <EmptyContent v-if="auth.session.value?.user.role === 'teacher'" class="text-sm text-muted-foreground">Akses data kelas tersedia setelah penugasan guru dikonfigurasi.</EmptyContent>
    </Empty>
  </div>
</template>
