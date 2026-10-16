<script setup lang="ts">
import {
  Settings,
  Camera,
  CalendarDays,
  ClipboardCheck,
  FileDown,
  GraduationCap,
  LayoutDashboard,
  ListChecks,
  School,
  ScrollText,
  ScanFace,
  Shield,
  Users,
} from '@lucide/vue'
import NavMain from '~/components/NavMain.vue'
import NavUser from '~/components/NavUser.vue'

const auth = useAuth()
const config = useRuntimeConfig()
const links = computed(() => [
  { title: 'Ringkasan', url: '/admin', icon: LayoutDashboard, category: 'Utama' },
  ...(auth.session.value?.permissions.includes('biometrics.enroll') ||
  auth.session.value?.permissions.includes('biometrics.revoke')
    ? [{ title: 'Wajah siswa', url: '/admin/biometrics', icon: ScanFace, category: 'Kehadiran' }]
    : []),
  ...(auth.has('academics.read')
    ? [
        {
          title: 'Data akademik',
          url: '/admin/academics',
          icon: GraduationCap,
          category: 'Akademik',
        },
      ]
    : []),
  ...(auth.has('academics.read')
    ? [
        {
          title: 'Jadwal & kalender',
          url: '/admin/schedules',
          icon: CalendarDays,
          category: 'Kehadiran',
        },
      ]
    : []),
  ...(auth.has('students.read')
    ? [{ title: 'Siswa', url: '/admin/students', icon: Users, category: 'Akademik' }]
    : []),
  ...(auth.has('attendance.read', 'attendance.manual')
    ? [
        {
          title: 'Sesi absensi',
          url: '/admin/attendance',
          icon: ClipboardCheck,
          category: 'Kehadiran',
        },
      ]
    : []),
  ...(auth.has('corrections.request', 'attendance.approve', 'attendance.manual')
    ? [
        {
          title: 'Koreksi absensi',
          url: '/admin/corrections',
          icon: ListChecks,
          category: 'Kehadiran',
        },
      ]
    : []),
  ...(auth.has('attendance.read', 'reports.export')
    ? [{ title: 'Laporan kehadiran', url: '/admin/reports', icon: FileDown, category: 'Kehadiran' }]
    : []),
  ...(auth.has('devices.health')
    ? [
        {
          title: 'Perangkat absensi',
          url: '/admin/devices',
          icon: Camera,
          category: 'Administrasi',
        },
      ]
    : []),
  { title: 'Pengaturan', url: '/admin/settings', icon: Settings, category: 'Administrasi' },
  ...(auth.session.value?.user.role === 'super_admin'
    ? [
        ...(auth.session.value.permissions.includes('users.read') ||
        auth.session.value.permissions.includes('users.write')
          ? [
              {
                title: 'Pengguna & peran',
                url: '/admin/users',
                icon: Users,
                category: 'Administrasi',
              },
            ]
          : []),
        ...(auth.session.value.permissions.includes('roles.manage')
          ? [
              {
                title: 'Peran & akses',
                url: '/admin/roles',
                icon: Shield,
                category: 'Administrasi',
              },
            ]
          : []),
        ...(auth.session.value.permissions.includes('audit.read')
          ? [
              {
                title: 'Riwayat aktivitas',
                url: '/admin/audit',
                icon: ScrollText,
                category: 'Administrasi',
              },
            ]
          : []),
      ]
    : []),
])
const groups = computed(() =>
  ['Utama', 'Akademik', 'Kehadiran', 'Administrasi']
    .map((title) => ({ title, items: links.value.filter((item) => item.category === title) }))
    .filter((group) => group.items.length),
)
</script>

<template>
  <Sidebar collapsible="offcanvas">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            as-child
            size="lg"
          >
            <NuxtLink to="/admin">
              <div
                class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground"
              >
                <img
                  v-if="auth.session.value?.school.logo_url"
                  :src="`${config.public.apiBase}${auth.session.value.school.logo_url}`"
                  alt=""
                  class="size-9 rounded-lg object-contain"
                />
                <School
                  v-else
                  class="size-5"
                />
              </div>
              <div class="grid min-w-0 flex-1 gap-0.5 text-left">
                <span class="truncate text-sm font-semibold">
                  {{ auth.session.value?.school.app_name || 'Absensi Sekolah' }}
                </span>
                <span class="truncate text-xs text-muted-foreground">
                  {{ auth.session.value?.school.name }}
                </span>
              </div>
            </NuxtLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent>
      <NavMain :groups="groups" />
    </SidebarContent>
    <SidebarFooter>
      <NavUser />
    </SidebarFooter>
    <SidebarRail aria-label="Buka atau tutup navigasi" />
  </Sidebar>
</template>
