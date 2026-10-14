<script setup lang="ts">
import { ClipboardCheck, ScrollText, Users } from '@lucide/vue'
import type { Result } from '~/types/auth'
import type { AttendanceDashboard } from '~/types/reports'

definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
const request = useApi()
const canView = computed(() =>
  ['super_admin', 'admin', 'teacher'].includes(auth.session.value?.user.role ?? ''),
)
const summary = ref<AttendanceDashboard | null>(null)
const message = ref('')
if (canView.value) {
  try {
    summary.value = (await request<Result<AttendanceDashboard>>('/dashboard/summary')).data
  } catch (error) {
    message.value = apiMessage(error)
  }
}
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Ringkasan kehadiran"
      description="Kehadiran hari ini dan tren tujuh hari, sesuai kelas yang dapat Anda akses."
    />
    <Alert
      v-if="route.query.forbidden"
      variant="destructive"
    >
      <AlertDescription>
        Halaman tersebut hanya tersedia untuk pengguna yang memiliki izin.
      </AlertDescription>
    </Alert>
    <Alert
      v-if="message"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <template v-if="summary">
      <SectionCards :totals="summary.totals" />
      <AttendanceTrendChart
        :days="summary.trend"
        :filters="summary.filters"
      />
    </template>
    <Card
      v-if="
        auth.session.value?.permissions.includes('attendance.read') ||
        auth.session.value?.permissions.includes('attendance.manual')
      "
    >
      <CardHeader>
        <CardTitle>Pengelolaan absensi</CardTitle>
        <CardDescription>
          Catat kehadiran dan tinjau koreksi. Alfa menunggu penutupan sesi dan rekonsiliasi.
        </CardDescription>
      </CardHeader>
      <CardContent class="flex flex-wrap gap-3">
        <Button as-child>
          <NuxtLink to="/admin/attendance">
            <ClipboardCheck />
            Buka sesi absensi
          </NuxtLink>
        </Button>
        <Button
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/corrections">Antrean koreksi</NuxtLink>
        </Button>
        <Button
          v-if="auth.session.value?.permissions.includes('academics.write')"
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/students">
            <Users />
            Kelola siswa
          </NuxtLink>
        </Button>
        <Button
          v-if="auth.session.value?.user.role === 'super_admin'"
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/audit">
            <ScrollText />
            Audit aktivitas
          </NuxtLink>
        </Button>
      </CardContent>
    </Card>
  </div>
</template>
