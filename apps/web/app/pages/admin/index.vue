<script setup lang="ts">
import { ClipboardCheck, ScrollText, Users } from '@lucide/vue'
import SectionCards from '~/components/SectionCards.vue'
definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
</script>

<template>
  <div class="flex flex-col gap-6">
    <div><h1 class="text-2xl font-semibold tracking-tight">Ringkasan sekolah</h1><p class="mt-2 text-sm text-muted-foreground">Ringkasan data akademik dan pengelolaan sekolah Anda.</p></div>
    <Alert v-if="route.query.forbidden" variant="destructive" role="alert"><AlertDescription>Halaman tersebut hanya tersedia untuk pengguna yang memiliki izin.</AlertDescription></Alert>
    <SectionCards v-if="auth.session.value?.permissions.includes('academics.read')" />
    <Card v-if="auth.session.value?.permissions.includes('academics.write')" class="shadow-none">
      <CardHeader><CardTitle>Pengelolaan sekolah</CardTitle><CardDescription>Mulai dari tahun ajaran dan kelas, kemudian tambahkan siswa dan penugasan guru.</CardDescription></CardHeader>
      <CardContent class="flex flex-wrap gap-3">
        <Button as-child><NuxtLink to="/admin/academics">Data akademik</NuxtLink></Button><Button as-child variant="outline"><NuxtLink to="/admin/students"><Users />Kelola siswa</NuxtLink></Button>
        <Button v-if="auth.session.value?.user.role === 'super_admin'" as-child variant="outline"><NuxtLink to="/admin/audit"><ScrollText />Lihat audit aktivitas</NuxtLink></Button>
      </CardContent>
    </Card>
    <Card v-if="auth.session.value?.permissions.includes('attendance.read') || auth.session.value?.permissions.includes('attendance.manual')"><CardHeader><CardTitle>Absensi sekolah</CardTitle><CardDescription>Kelola sesi, catat kehadiran manual dan tinjau koreksi. Alfa menunggu penutupan sesi dan rekonsiliasi.</CardDescription></CardHeader><CardContent class="flex flex-wrap gap-3"><Button as-child><NuxtLink to="/admin/attendance"><ClipboardCheck />Buka sesi absensi</NuxtLink></Button><Button as-child variant="outline"><NuxtLink to="/admin/corrections">Antrean koreksi</NuxtLink></Button></CardContent></Card>
  </div>
</template>
