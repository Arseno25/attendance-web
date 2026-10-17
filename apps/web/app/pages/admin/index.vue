<script setup lang="ts">
import { ClipboardCheck, ScrollText, Users } from '@lucide/vue'
import { statusVariant, recognitionStatusText } from '~/lib/status'
import { attendanceLabels } from '~/types/attendance'
import type { Result } from '~/types/auth'
import type { AttendanceDashboard } from '~/types/reports'

definePageMeta({ layout: 'admin', middleware: ['auth'] })
const auth = useAuth()
const route = useRoute()
const request = useApi()
const { dateTime } = useReports()
const canView = computed(() => auth.has('attendance.read'))
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
      :title="canView ? 'Ringkasan kehadiran' : 'Panel sekolah'"
      :description="
        canView
          ? 'Kehadiran hari ini dan tren tujuh hari, sesuai kelas yang dapat Anda akses.'
          : 'Pilih menu di sidebar sesuai tugas Anda.'
      "
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
      <div
        v-if="summary.operations"
        class="grid gap-4 @xl/main:grid-cols-3"
      >
        <Card>
          <CardHeader>
            <CardDescription>Sesi absensi berjalan</CardDescription>
            <CardTitle class="text-2xl font-semibold tabular-nums">
              {{ summary.operations.open_sessions }}
            </CardTitle>
          </CardHeader>
          <CardFooter class="text-sm text-muted-foreground">
            Sesi terbuka yang saat ini menerima kehadiran.
          </CardFooter>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Perangkat kamera</CardDescription>
            <CardTitle class="text-2xl font-semibold tabular-nums">
              {{ summary.operations.online_devices }} / {{ summary.operations.total_devices }}
            </CardTitle>
          </CardHeader>
          <CardFooter class="text-sm text-muted-foreground">
            Kamera yang aktif dalam 5 menit terakhir ·
            {{ summary.operations.revoked_devices }} perangkat telah dinonaktifkan.
          </CardFooter>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Absensi wajah otomatis</CardDescription>
            <CardTitle class="flex items-center gap-2 text-lg font-semibold">
              <Badge :variant="summary.operations.recognition_enabled ? 'success' : 'secondary'">
                {{
                  summary.operations.recognition_enabled ? 'Otomatis aktif' : 'Verifikasi manual'
                }}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardFooter class="text-sm text-muted-foreground">
            {{ recognitionStatusText(summary.operations) }}
          </CardFooter>
        </Card>
      </div>
      <AttendanceTrendChart
        :days="summary.trend"
        :filters="summary.filters"
      />
      <Card class="min-w-0">
        <CardHeader>
          <CardTitle>Aktivitas absensi terbaru</CardTitle>
          <CardDescription>
            10 pembaruan kehadiran terakhir pada sesi yang dapat Anda akses.
          </CardDescription>
        </CardHeader>
        <CardContent class="min-w-0">
          <DataTable
            class="min-w-[640px]"
            :data="summary.recent ?? []"
            :columns="[
              { id: 'siswa', label: 'Siswa', cellClass: 'font-medium' },
              { id: 'kelas', label: 'Kelas & sesi' },
              { id: 'status', label: 'Status' },
              { id: 'waktu', label: 'Waktu tercatat' },
            ]"
            caption="Aktivitas absensi terbaru"
            row-key="student_id"
          >
            <template #cell-siswa="{ item }">
              {{ item.full_name }}
              <div class="text-xs text-muted-foreground">{{ item.nis }}</div>
            </template>
            <template #cell-kelas="{ item }">
              {{ item.class_name }} · {{ item.session_name }}
              <div class="text-xs text-muted-foreground">{{ item.date }}</div>
            </template>
            <template #cell-status="{ item }">
              <Badge :variant="statusVariant(item.status)">
                {{ attendanceLabels[item.status] ?? item.status }}
              </Badge>
            </template>
            <template #cell-waktu="{ item }">
              {{ dateTime(item.check_out_at ?? item.check_in_at ?? item.updated_at) }}
            </template>
            <template #empty>
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>Belum ada aktivitas kehadiran</EmptyTitle>
                  <EmptyDescription>
                    Kehadiran yang tercatat hari ini akan muncul secara otomatis di sini.
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            </template>
          </DataTable>
        </CardContent>
      </Card>
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
          Catat kehadiran dan tinjau koreksi. Status alfa ditetapkan setelah sesi ditutup.
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
          v-if="auth.has('corrections.request', 'attendance.approve', 'attendance.manual')"
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/corrections">Antrean koreksi</NuxtLink>
        </Button>
        <Button
          v-if="auth.has('students.read')"
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/students">
            <Users />
            Kelola siswa
          </NuxtLink>
        </Button>
        <Button
          v-if="auth.session.value?.user.role === 'super_admin' && auth.has('audit.read')"
          as-child
          variant="outline"
        >
          <NuxtLink to="/admin/audit">
            <ScrollText />
            Riwayat aktivitas
          </NuxtLink>
        </Button>
      </CardContent>
    </Card>
  </div>
</template>
