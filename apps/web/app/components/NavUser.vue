<script setup lang="ts">
import { toast } from 'vue-sonner'
import { ChevronsUpDown, LogOut, UserRound } from '@lucide/vue'
import { useSidebar } from '~/components/ui/sidebar'
import { roleLabels } from '~/types/auth'

const auth = useAuth()
const { isMobile } = useSidebar()
const busy = ref(false)
const user = computed(() => auth.session.value?.user)
const initials = computed(() =>
  user.value?.full_name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase(),
)
async function logout() {
  if (busy.value) return
  busy.value = true
  try {
    await auth.logout()
    toast.success('Anda telah keluar.')
  } catch (error) {
    toast.error(apiMessage(error))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <SidebarMenu v-if="user">
    <SidebarMenuItem>
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <SidebarMenuButton
            size="lg"
            aria-label="Menu akun"
            :disabled="busy"
            class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
          >
            <Avatar class="size-8 rounded-lg">
              <AvatarFallback class="rounded-lg">{{ initials }}</AvatarFallback>
            </Avatar>
            <div class="grid min-w-0 flex-1 text-left text-sm leading-tight">
              <span class="truncate font-medium">{{ user.full_name }}</span>
              <span class="truncate text-xs text-muted-foreground">
                {{ roleLabels[user.role] }}
              </span>
            </div>
            <ChevronsUpDown class="ml-auto size-4" />
          </SidebarMenuButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          class="min-w-64"
          :side="isMobile ? 'bottom' : 'right'"
          align="end"
          :side-offset="4"
        >
          <DropdownMenuLabel class="grid gap-1 font-normal">
            <span class="font-medium">{{ user.full_name }}</span>
            <span class="text-xs text-muted-foreground">{{ user.email }}</span>
            <Badge
              variant="secondary"
              class="mt-1 w-fit"
            >
              {{ roleLabels[user.role] }}
            </Badge>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem as-child>
            <NuxtLink to="/admin/account">
              <UserRound />
              Akun saya
            </NuxtLink>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            :disabled="busy"
            @select="logout"
          >
            <LogOut />
            {{ busy ? 'Keluar…' : 'Keluar' }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
