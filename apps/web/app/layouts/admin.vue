<script setup lang="ts">
import { roleLabels } from '~/types/auth'

const auth = useAuth()
const route = useRoute()
const message = ref('')
const loggingOut = ref(false)
const links = computed(() => [
  { to: '/admin', label: 'Ringkasan' },
  ...(auth.session.value?.user.role === 'super_admin'
    ? [{ to: '/admin/users', label: 'Pengguna & peran' }, { to: '/admin/audit', label: 'Audit aktivitas' }]
    : []),
])
async function logout() {
  loggingOut.value = true
  try { await auth.logout() }
  catch (error) { message.value = apiMessage(error) }
  finally { loggingOut.value = false }
}
</script>

<template>
  <div>
    <header class="border-b">
      <div class="flex flex-wrap items-center justify-between gap-4 px-6 py-4">
        <NuxtLink to="/admin" class="text-lg font-semibold tracking-tight">Absensi Sekolah</NuxtLink>
        <div v-if="auth.session.value" class="flex items-center gap-4">
          <div class="text-right text-sm">
            <p class="font-medium">{{ auth.session.value.user.full_name }}</p>
            <p class="text-muted-foreground">{{ roleLabels[auth.session.value.user.role] }}</p>
          </div>
          <Button variant="outline" :disabled="loggingOut" @click="logout">{{ loggingOut ? 'Keluar…' : 'Keluar' }}</Button>
        </div>
      </div>
    </header>
    <div class="lg:grid lg:min-h-[calc(100vh-73px)] lg:grid-cols-[240px_1fr]">
      <aside class="border-b bg-muted/30 px-4 py-5 lg:border-b-0 lg:border-r">
        <p class="mb-5 hidden px-3 text-sm text-muted-foreground lg:block">{{ auth.session.value?.school.name }}</p>
        <nav aria-label="Navigasi admin" class="flex flex-wrap gap-1 lg:flex-col">
          <NuxtLink v-for="link in links" :key="link.to" :to="link.to" :aria-current="route.path === link.to ? 'page' : undefined" :class="['rounded-md px-3 py-2 text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring', route.path === link.to ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent']">{{ link.label }}</NuxtLink>
        </nav>
      </aside>
      <main class="min-w-0 px-6 py-8 lg:px-10">
        <Alert v-if="message" class="mb-6" role="alert"><AlertDescription>{{ message }}</AlertDescription></Alert>
        <slot />
      </main>
    </div>
  </div>
</template>
