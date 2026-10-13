<script setup lang="ts">
const route = useRoute()
const title = computed(() => ({ '/admin': 'Ringkasan', '/admin/users': 'Pengguna & peran', '/admin/audit': 'Audit aktivitas', '/admin/academics': 'Data akademik', '/admin/students': 'Siswa' }[route.path] ?? (route.path.startsWith('/admin/students/') ? 'Detail siswa' : 'Panel sekolah')))
</script>

<template>
  <header class="flex h-(--header-height) shrink-0 items-center gap-2 border-b">
    <div class="flex min-w-0 flex-1 items-center gap-2 px-4 lg:px-6">
      <SidebarTrigger class="-ml-1" aria-label="Buka atau tutup navigasi" />
      <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
      <Breadcrumb aria-label="Lokasi halaman">
        <BreadcrumbList>
          <BreadcrumbItem class="hidden sm:block"><BreadcrumbLink as-child><NuxtLink to="/admin">Panel sekolah</NuxtLink></BreadcrumbLink></BreadcrumbItem>
          <BreadcrumbSeparator class="hidden sm:block" />
          <BreadcrumbItem><BreadcrumbPage>{{ title }}</BreadcrumbPage></BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
    </div>
  </header>
</template>
