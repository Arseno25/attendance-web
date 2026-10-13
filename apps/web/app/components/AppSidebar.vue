<script setup lang="ts">
import { LayoutDashboard, School, ScrollText, Users } from '@lucide/vue'
import NavMain from '~/components/NavMain.vue'
import NavUser from '~/components/NavUser.vue'

const auth = useAuth()
const links = computed(() => [
  { title: 'Ringkasan', url: '/admin', icon: LayoutDashboard },
  ...(auth.session.value?.user.role === 'super_admin'
    ? [{ title: 'Pengguna & peran', url: '/admin/users', icon: Users }, { title: 'Audit aktivitas', url: '/admin/audit', icon: ScrollText }]
    : []),
])
</script>

<template>
  <Sidebar collapsible="offcanvas">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton as-child size="lg">
            <NuxtLink to="/admin">
              <div class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground"><School class="size-5" /></div>
              <div class="grid min-w-0 flex-1 gap-0.5 text-left">
                <span class="truncate text-sm font-semibold">Absensi Sekolah</span>
                <span class="truncate text-xs text-muted-foreground">{{ auth.session.value?.school.name }}</span>
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
