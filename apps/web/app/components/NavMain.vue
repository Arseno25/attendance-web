<script setup lang="ts">
import type { Component } from 'vue'
import { useSidebar } from '~/components/ui/sidebar'

defineProps<{
  groups: { title: string; items: { title: string; url: string; icon: Component }[] }[]
}>()
const route = useRoute()
const { setOpenMobile } = useSidebar()
watch(
  () => route.path,
  () => setOpenMobile(false),
)
</script>

<template>
  <nav aria-label="Navigasi admin">
    <SidebarGroup
      v-for="group in groups"
      :key="group.title"
    >
      <SidebarGroupLabel>{{ group.title }}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          <SidebarMenuItem
            v-for="item in group.items"
            :key="item.url"
          >
            <SidebarMenuButton
              as-child
              :is-active="
                route.path === item.url ||
                (item.url !== '/admin' && route.path.startsWith(`${item.url}/`))
              "
              :tooltip="item.title"
            >
              <NuxtLink
                :to="item.url"
                :aria-current="
                  route.path === item.url ||
                  (item.url !== '/admin' && route.path.startsWith(`${item.url}/`))
                    ? 'page'
                    : undefined
                "
              >
                <component :is="item.icon" />
                <span>{{ item.title }}</span>
              </NuxtLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  </nav>
</template>
