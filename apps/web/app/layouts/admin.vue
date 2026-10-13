<script setup lang="ts">
import AppSidebar from '~/components/AppSidebar.vue'
import SiteHeader from '~/components/SiteHeader.vue'
</script>

<template>
  <SidebarProvider :style="{ '--sidebar-width': 'calc(var(--spacing) * 72)', '--header-height': 'calc(var(--spacing) * 12)' }">
    <AppSidebar variant="inset" />
    <SidebarInset class="min-w-0">
      <SiteHeader />
      <div id="panel-content" class="@container/main flex min-w-0 flex-1 flex-col gap-4 px-4 py-4 md:gap-6 md:py-6 lg:px-6">
        <slot />
      </div>
    </SidebarInset>
  </SidebarProvider>
</template>
