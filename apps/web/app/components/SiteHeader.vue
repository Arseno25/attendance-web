<script setup lang="ts">
const route = useRoute()
const title = computed(
  () =>
    ({
      '/admin': 'Ringkasan',
      '/admin/account': 'Akun saya',
      '/admin/biometrics': 'Wajah siswa',
      '/admin/students/import': 'Import siswa',
      '/admin/devices': 'Perangkat kiosk',
      '/admin/users': 'Pengguna & peran',
      '/admin/audit': 'Audit aktivitas',
      '/admin/reports': 'Laporan kehadiran',
      '/admin/academics': 'Data akademik',
      '/admin/students': 'Siswa',
      '/admin/schedules': 'Jadwal & kalender',
      '/admin/attendance': 'Sesi absensi',
      '/admin/corrections': 'Koreksi absensi',
    })[route.path] ??
    (route.path.startsWith('/admin/students/')
      ? 'Detail siswa'
      : route.path.startsWith('/admin/attendance/')
        ? 'Detail sesi'
        : 'Panel sekolah'),
)
</script>

<template>
  <header class="flex h-(--header-height) shrink-0 items-center gap-2 border-b">
    <div class="flex min-w-0 flex-1 items-center gap-2 px-4 lg:px-6">
      <SidebarTrigger
        class="-ml-1"
        aria-label="Buka atau tutup navigasi"
      />
      <Separator
        orientation="vertical"
        class="mx-2 data-[orientation=vertical]:h-4"
      />
      <Breadcrumb aria-label="Lokasi halaman">
        <BreadcrumbList>
          <BreadcrumbItem class="hidden sm:block">
            <BreadcrumbLink as-child><NuxtLink to="/admin">Panel sekolah</NuxtLink></BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator class="hidden sm:block" />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ title }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
    </div>
    <div class="ml-auto shrink-0 pr-4 lg:pr-6"><ModeToggle /></div>
  </header>
</template>
