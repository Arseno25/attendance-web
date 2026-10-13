<script setup lang="ts">
import AppSidebar from '~/components/AppSidebar.vue'
import SiteHeader from '~/components/SiteHeader.vue'
</script>

<template>
  <SidebarProvider :style="{ '--sidebar-width': 'calc(var(--spacing) * 72)', '--header-height': 'calc(var(--spacing) * 14)' }">
    <AppSidebar variant="inset" />
    <SidebarInset class="min-w-0">
      <SiteHeader />
      <div id="panel-content" class="@container/main flex min-w-0 flex-1 flex-col gap-6 p-4 md:p-6">
        <slot />
      </div>
    </SidebarInset>
  </SidebarProvider>
</template>
