<script setup lang="ts">
import {
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
  Users,
} from '@lucide/vue'
import NavMain from '~/components/NavMain.vue'
import NavUser from '~/components/NavUser.vue'

const auth = useAuth()
const links = computed(() => [
  { title: 'Ringkasan', url: '/admin', icon: LayoutDashboard },
  ...(auth.session.value?.permissions.includes('biometrics.enroll') ||
  auth.session.value?.permissions.includes('biometrics.revoke')
    ? [{ title: 'Wajah siswa', url: '/admin/biometrics', icon: ScanFace }]
    : []),
  ...(auth.session.value?.permissions.includes('academics.read')
    ? [
        { title: 'Data akademik', url: '/admin/academics', icon: GraduationCap },
        { title: 'Siswa', url: '/admin/students', icon: Users },
      ]
    : []),
  ...(auth.session.value?.permissions.includes('academics.read')
    ? [{ title: 'Jadwal & kalender', url: '/admin/schedules', icon: CalendarDays }]
    : []),
  ...(auth.session.value?.permissions.includes('attendance.read') ||
  auth.session.value?.permissions.includes('attendance.manual')
    ? [
        { title: 'Sesi absensi', url: '/admin/attendance', icon: ClipboardCheck },
        { title: 'Koreksi absensi', url: '/admin/corrections', icon: ListChecks },
      ]
    : []),
  ...(['super_admin', 'admin', 'teacher'].includes(auth.session.value?.user.role ?? '')
    ? [{ title: 'Laporan kehadiran', url: '/admin/reports', icon: FileDown }]
    : []),
  ...(auth.session.value?.permissions.includes('devices.health')
    ? [{ title: 'Perangkat kiosk', url: '/admin/devices', icon: Camera }]
    : []),
  ...(auth.session.value?.user.role === 'super_admin'
    ? [
        { title: 'Pengguna & peran', url: '/admin/users', icon: Users },
        { title: 'Audit aktivitas', url: '/admin/audit', icon: ScrollText },
      ]
    : []),
])
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
                <School class="size-5" />
              </div>
              <div class="grid min-w-0 flex-1 gap-0.5 text-left">
                <span class="truncate text-sm font-semibold">Absensi Sekolah</span>
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
      <NavMain :items="links" />
    </SidebarContent>
    <SidebarFooter>
      <NavUser />
    </SidebarFooter>
    <SidebarRail aria-label="Buka atau tutup navigasi" />
  </Sidebar>
</template>
